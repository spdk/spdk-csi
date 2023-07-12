/*
Copyright (c) Arm Limited and Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spdk

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog"

	csicommon "github.com/spdk/spdk-csi/pkg/csi-common"
	"github.com/spdk/spdk-csi/pkg/util"
)

var errVolumeInCreation = status.Error(codes.Internal, "volume in creation")

type controllerServer struct {
	*csicommon.DefaultControllerServer
	spdkNodeConfigs map[string]*util.SpdkNodeConfig
	volumeLocks     *util.VolumeLocks
}

type spdkVolume struct {
	lvolID   string
	nodeName string
}

func (cs *controllerServer) CreateVolume(_ context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	volumeID := req.GetName()
	unlock := cs.volumeLocks.Lock(volumeID)
	defer unlock()

	csiVolume, err := cs.createVolume(req)
	if err != nil {
		klog.Errorf("failed to create volume, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	volumeInfo, err := cs.publishVolume(csiVolume.GetVolumeId(), req.Secrets)
	if err != nil {
		klog.Errorf("failed to publish volume, volumeID: %s err: %v", volumeID, err)
		cs.deleteVolume(csiVolume.GetVolumeId(), req.Secrets) //nolint:errcheck // we can do little
		return nil, status.Error(codes.Internal, err.Error())
	}
	// copy volume info. node needs these info to contact target(ip, port, nqn, ...)
	if csiVolume.VolumeContext == nil {
		csiVolume.VolumeContext = volumeInfo
	} else {
		for k, v := range volumeInfo {
			csiVolume.VolumeContext[k] = v
		}
	}

	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (cs *controllerServer) DeleteVolume(_ context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	unlock := cs.volumeLocks.Lock(volumeID)
	defer unlock()
	// no harm if volume already unpublished
	err := cs.unpublishVolume(volumeID, req.Secrets)
	switch {
	case errors.Is(err, util.ErrVolumeUnpublished):
		// unpublished but not deleted in last request?
		klog.Warningf("volume not published: %s", volumeID)
	case errors.Is(err, util.ErrVolumeDeleted):
		// deleted in previous request?
		klog.Warningf("volume already deleted: %s", volumeID)
		return &csi.DeleteVolumeResponse{}, nil
	case err != nil:
		klog.Errorf("failed to unpublish volume, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// no harm if volume already deleted
	err = cs.deleteVolume(volumeID, req.Secrets)
	if errors.Is(err, util.ErrJSONNoSuchDevice) {
		// deleted in previous request?
		klog.Warningf("volume not exists: %s", volumeID)
	} else if err != nil {
		klog.Errorf("failed to delete volume, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(_ context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// make sure we support all requested caps
	for _, cap := range req.VolumeCapabilities {
		supported := false
		for _, accessMode := range cs.Driver.GetVolumeCapabilityAccessModes() {
			if cap.GetAccessMode().GetMode() == accessMode.GetMode() {
				supported = true
				break
			}
		}
		if !supported {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: ""}, nil
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.VolumeCapabilities,
		},
	}, nil
}

func (cs *controllerServer) CreateSnapshot(_ context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	volumeID := req.GetSourceVolumeId()
	unlock := cs.volumeLocks.Lock(volumeID)
	defer unlock()

	snapshotName := req.GetName()
	spdkVol, err := getSPDKVol(volumeID)
	if err != nil {
		klog.Errorf("failed to get spdk volume, volumeID: %s err: %v", volumeID, err)
		return nil, err
	}

	node, err := cs.getSpdkNode(spdkVol.nodeName, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	snapshotID, err := node.CreateSnapshot(spdkVol.lvolID, snapshotName)
	if err != nil {
		klog.Errorf("failed to create snapshot, volumeID: %s snapshotName: %s err: %v", volumeID, snapshotName, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	volInfo, err := node.VolumeInfo(spdkVol.lvolID)
	if err != nil {
		klog.Errorf("failed to get volume info, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	size, err := strconv.ParseInt(volInfo["lvolSize"], 10, 64)
	if err != nil {
		klog.Errorf("failed to parse volume size, lvolSize: %s err: %v", volInfo["lvolSize"], err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	creationTime := timestamppb.Now()
	snapshotData := csi.Snapshot{
		SizeBytes:      size,
		SnapshotId:     fmt.Sprintf("%s:%s", spdkVol.nodeName, snapshotID),
		SourceVolumeId: spdkVol.lvolID,
		CreationTime:   creationTime,
		ReadyToUse:     true,
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &snapshotData,
	}, nil
}

func (cs *controllerServer) DeleteSnapshot(_ context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	snapshotID := req.GetSnapshotId()
	unlock := cs.volumeLocks.Lock(snapshotID)
	defer unlock()

	spdkVol, err := getSPDKVol(snapshotID)
	if err != nil {
		klog.Errorf("failed to get spdk volume, snapshotID: %s err: %v", snapshotID, err)
		return nil, err
	}

	node, err := cs.getSpdkNode(spdkVol.nodeName, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	err = node.DeleteVolume(spdkVol.lvolID)
	if err != nil {
		klog.Errorf("failed to delete snapshot, snapshotID: %s err: %v", snapshotID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (cs *controllerServer) createVolume(req *csi.CreateVolumeRequest) (*csi.Volume, error) {
	size := req.GetCapacityRange().GetRequiredBytes()
	if size == 0 {
		klog.Warningln("invalid volume size, resize to 1G")
		size = 1024 * 1024 * 1024
	}
	sizeMiB := util.ToMiB(size)
	vol := csi.Volume{
		CapacityBytes: sizeMiB * 1024 * 1024,
		VolumeContext: req.GetParameters(),
		ContentSource: req.GetVolumeContentSource(),
	}

	volumeID, err := cs.getVolume(req)
	if err == nil {
		vol.VolumeId = volumeID
		return &vol, nil
	}
	var lvolID string

	if req.GetVolumeContentSource() != nil {
		// if volume content source is specified, using the same node/lvstore as the source volume.
		// find the node/lvstore of the specified content source volume
		nodeName, lvstore, sourceLvolID, err2 := cs.getSnapshotInfo(req.GetVolumeContentSource(), req.Secrets)
		if err2 != nil {
			return nil, err2
		}
		node, err2 := cs.getSpdkNode(nodeName, req.Secrets)
		if err2 != nil {
			return nil, status.Error(codes.Internal, err2.Error())
		}
		// create a volume cloned from the source volume
		lvolID, err = node.CloneVolume(req.GetName(), lvstore, sourceLvolID)
		vol.VolumeId = fmt.Sprintf("%s:%s", nodeName, lvolID)
		if err != nil {
			return nil, err
		}
		return &vol, nil
	}
	// schedule a SPDK node/lvstore to create the volume.
	// schedule suitable node:lvstore
	nodeName, lvstore, err2 := cs.schedule(sizeMiB, req.Secrets)
	if err2 != nil {
		return nil, err2
	}
	node, err2 := cs.getSpdkNode(nodeName, req.Secrets)
	if err2 != nil {
		return nil, status.Error(codes.Internal, err2.Error())
	}
	// TODO: re-schedule on ErrJSONNoSpaceLeft per optimistic concurrency control
	// create a new volume
	lvolID, err = node.CreateVolume(req.GetName(), lvstore, sizeMiB)
	// in the subsequent DeleteVolume() request, a nodeName needs to be specified,
	// but the current CSI mechanism only passes the VolumeId to DeleteVolume().
	// therefore, the nodeName is included as part of the VolumeId.
	vol.VolumeId = fmt.Sprintf("%s:%s", nodeName, lvolID)
	if err != nil {
		return nil, err
	}
	return &vol, nil
}

func (cs *controllerServer) getVolume(req *csi.CreateVolumeRequest) (string, error) {
	// check all SPDK nodes to see if the volume has already been created
	for _, cfg := range cs.spdkNodeConfigs {
		node, err := cs.getSpdkNode(cfg.Name, req.Secrets)
		if err != nil {
			return "nil", fmt.Errorf("failed to get spdkNode %s: %s", cfg.Name, err.Error())
		}
		lvStores, err := node.LvStores()
		if err != nil {
			return "", fmt.Errorf("get lvstores of node:%s failed: %w", cfg.Name, err)
		}
		for lvsIdx := range lvStores {
			volumeID, err := node.GetVolume(req.GetName(), lvStores[lvsIdx].Name)
			if err == nil {
				return fmt.Sprintf("%s:%s", cfg.Name, volumeID), nil
			}
		}
	}
	return "", fmt.Errorf("volume not found")
}

func getSPDKVol(csiVolumeID string) (*spdkVolume, error) {
	// extract spdkNodeName and spdkLvolID from csiVolumeID
	// csiVolumeID: node001:8e2dcb9d-3a79-4362-965e-fdb0cd3f4b8d
	// spdkNodeName: node001
	// spdklvolID: 8e2dcb9d-3a79-4362-965e-fdb0cd3f4b8d

	ids := strings.Split(csiVolumeID, ":")
	if len(ids) == 2 {
		return &spdkVolume{
			nodeName: ids[0],
			lvolID:   ids[1],
		}, nil
	}
	return nil, fmt.Errorf("missing nodeName in volume: %s", csiVolumeID)
}

func (cs *controllerServer) publishVolume(volumeID string, secrets map[string]string) (map[string]string, error) {
	spdkVol, err := getSPDKVol(volumeID)
	if err != nil {
		return nil, err
	}
	node, err := cs.getSpdkNode(spdkVol.nodeName, secrets)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	err = node.PublishVolume(spdkVol.lvolID)
	if err != nil {
		return nil, err
	}

	volumeInfo, err := node.VolumeInfo(spdkVol.lvolID)
	if err != nil {
		cs.unpublishVolume(volumeID, secrets) //nolint:errcheck // we can do little
		return nil, err
	}
	return volumeInfo, nil
}

func (cs *controllerServer) deleteVolume(volumeID string, secrets map[string]string) error {
	spdkVol, err := getSPDKVol(volumeID)
	if err != nil {
		return err
	}
	node, err := cs.getSpdkNode(spdkVol.nodeName, secrets)
	if err != nil {
		return err
	}
	return node.DeleteVolume(spdkVol.lvolID)
}

func (cs *controllerServer) unpublishVolume(volumeID string, secrets map[string]string) error {
	spdkVol, err := getSPDKVol(volumeID)
	if err != nil {
		return err
	}
	node, err := cs.getSpdkNode(spdkVol.nodeName, secrets)
	if err != nil {
		return err
	}
	return node.UnpublishVolume(spdkVol.lvolID)
}

func (cs *controllerServer) getSnapshotInfo(vcs *csi.VolumeContentSource, secrets map[string]string) (
	nodeName, lvstore, sourceLvolID string, err error,
) {
	snapshotSource := vcs.GetSnapshot()

	if snapshotSource == nil {
		err = fmt.Errorf("invalid volume content source, only snapshot source is supported")
		return
	}
	snapSpdkVol, err := getSPDKVol(snapshotSource.GetSnapshotId())
	if err != nil {
		return
	}
	nodeName = snapSpdkVol.nodeName
	sourceLvolID = snapSpdkVol.lvolID

	node, err := cs.getSpdkNode(nodeName, secrets)
	if err != nil {
		return
	}

	sourceLvolInfo, err := node.VolumeInfo(sourceLvolID)
	if err != nil {
		return
	}
	lvstore = sourceLvolInfo["lvstore"]
	return
}

// simplest volume scheduler: find first node:lvstore with enough free space
func (cs *controllerServer) schedule(sizeMiB int64, secrets map[string]string) (nodeName, lvstore string, err error) {
	for _, cfg := range cs.spdkNodeConfigs {
		spdkNode, err := cs.getSpdkNode(cfg.Name, secrets)
		if err != nil {
			klog.Errorf("failed to get spdkNode %s: %s", nodeName, err.Error())
			continue
		}
		lvstores, err := spdkNode.LvStores()
		if err != nil {
			klog.Errorf("failed to get lvstores from node %s: %s", spdkNode.Info(), err.Error())
			continue
		}
		// check if lvstore has enough free space
		for i := range lvstores {
			lvstore := &lvstores[i]
			if lvstore.FreeSizeMiB > sizeMiB {
				return cfg.Name, lvstore.Name, nil
			}
		}
		klog.Infof("not enough free space from node %s", spdkNode.Info())
	}

	return "", "", fmt.Errorf("failed to find node with enough free space")
}

func (cs *controllerServer) getSpdkNode(nodeName string, secrets map[string]string) (util.SpdkNode, error) {
	spdkSecrets, err := util.NewSpdkSecrets(secrets["secret.json"])
	if err != nil {
		return nil, err
	}
	node, ok := cs.spdkNodeConfigs[nodeName]
	if !ok {
		return nil, fmt.Errorf("%s spdknode not exists", node.Name)
	}
	for i := range spdkSecrets.Tokens {
		token := spdkSecrets.Tokens[i]
		if token.Name == nodeName {
			spdkNode, err := util.NewSpdkNode(node.URL, token.UserName, token.Password, node.TargetType, node.TargetAddr)
			if err != nil {
				klog.Errorf("failed to create spdk node %s: %s", node.Name, err.Error())
			}
			return spdkNode, nil
		}
	}
	return nil, fmt.Errorf("failed to find secret for spdk node %s", node.Name)
}

func newControllerServer(d *csicommon.CSIDriver) (*controllerServer, error) {
	server := controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		spdkNodeConfigs:         map[string]*util.SpdkNodeConfig{},
		volumeLocks:             util.NewVolumeLocks(),
	}

	config, err := util.NewCSIControllerConfig("SPDKCSI_CONFIG", "/etc/spdkcsi-config/config.json")
	if err != nil {
		return nil, err
	}
	for i := range config.Nodes {
		server.spdkNodeConfigs[config.Nodes[i].Name] = &config.Nodes[i]
	}

	if len(server.spdkNodeConfigs) == 0 {
		return nil, fmt.Errorf("no valid spdk node found")
	}

	return &server, nil
}
