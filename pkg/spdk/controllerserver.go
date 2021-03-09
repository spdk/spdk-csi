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
	"fmt"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"

	csicommon "github.com/spdk/spdk-csi/pkg/csi-common"
	"github.com/spdk/spdk-csi/pkg/util"
)

var errVolumeInCreation = status.Error(codes.Internal, "volume in creation")

type controllerServer struct {
	*csicommon.DefaultControllerServer

	spdkNodes []util.SpdkNode // all spdk nodes in cluster

	volumes     map[string]*volume // volume id to volume struct
	volumesIdem map[string]string  // volume name to id, for CreateVolume idempotency
	mtx         sync.Mutex         // protect volumes and volumesIdem map
}

type volume struct {
	name      string // CO provided volume name
	spdkNode  util.SpdkNode
	csiVolume csi.Volume
	mtx       sync.Mutex // per volume lock to serialize DeleteVolume requests
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// be idempotent to duplicated requests
	volume, err := func() (*volume, error) {
		const creatingTag = "__CREATING__"
		cs.mtx.Lock()
		defer cs.mtx.Unlock()

		volumeID, exists := cs.volumesIdem[req.Name]
		if exists {
			// this is a duplicated request
			if volumeID == creatingTag {
				// another task is still processing same request
				return nil, errVolumeInCreation
			}
			// another task has successfully processed same request
			volume := cs.volumes[volumeID]
			klog.Warningf("volume exists: %s, %p", req.Name, volume)
			return volume, nil
		}
		// we're processing the first request
		cs.volumesIdem[req.Name] = creatingTag
		return nil, nil
	}()
	if err != nil {
		return nil, err
	}
	if volume != nil {
		return &csi.CreateVolumeResponse{Volume: &volume.csiVolume}, nil
	}

	// no concurrent task for same request from now on
	defer func() {
		if err != nil {
			cs.mtx.Lock()
			delete(cs.volumesIdem, req.Name)
			cs.mtx.Unlock()
		}
	}()

	volume, err = cs.createVolume(req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	volumeInfo, err := publishVolume(volume)
	if err != nil {
		deleteVolume(volume) // nolint:errcheck // we can do little
		return nil, status.Error(codes.Internal, err.Error())
	}
	// copy volume info. node needs these info to contact target(ip, port, nqn, ...)
	if volume.csiVolume.VolumeContext == nil {
		volume.csiVolume.VolumeContext = volumeInfo
	} else {
		for k, v := range volumeInfo {
			volume.csiVolume.VolumeContext[k] = v
		}
	}

	volumeID := volume.csiVolume.GetVolumeId()
	cs.mtx.Lock()
	cs.volumes[volumeID] = volume
	cs.volumesIdem[req.Name] = volumeID
	cs.mtx.Unlock()

	return &csi.CreateVolumeResponse{Volume: &volume.csiVolume}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	cs.mtx.Lock()
	volume, exists := cs.volumes[volumeID]
	cs.mtx.Unlock()
	if !exists {
		// already deleted?
		klog.Warningf("volume not exists: %s", volumeID)
		return &csi.DeleteVolumeResponse{}, nil
	} else if volume.csiVolume.GetVolumeId() != volumeID {
		return nil, status.Error(codes.Internal, "data corrupt! volume id mismatch!")
	}

	// serialize requests to same volume by holding volume lock
	volume.mtx.Lock()
	defer volume.mtx.Unlock()

	// no harm if volume already unpublished
	err := unpublishVolume(volume)
	switch {
	case err == util.ErrVolumeUnpublished:
		// unpublished but not deleted in last request?
		klog.Warningf("volume not published: %s", volumeID)
	case err == util.ErrVolumeDeleted:
		// deleted in previous request?
		klog.Warningf("volume already deleted: %s", volumeID)
	case err != nil:
		return nil, status.Error(codes.Internal, err.Error())
	}

	// no harm if volume already deleted
	err = deleteVolume(volume)
	if err == util.ErrJSONNoSuchDevice {
		// deleted in previous request?
		klog.Warningf("volume not exists: %s", volumeID)
	} else if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// no harm if volumeID already deleted
	cs.mtx.Lock()
	delete(cs.volumes, volumeID)
	delete(cs.volumesIdem, volume.name)
	cs.mtx.Unlock()

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
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

func (cs *controllerServer) createVolume(req *csi.CreateVolumeRequest) (*volume, error) {
	size := req.GetCapacityRange().GetRequiredBytes()
	if size == 0 {
		klog.Warningln("invalid volume size, resize to 1G")
		size = 1024 * 1024 * 1024
	}
	sizeMiB := util.ToMiB(size)

	// schedule suitable node:lvstore
	spdkNode, lvstore, err := cs.schedule(sizeMiB)
	if err != nil {
		return nil, err
	}

	// TODO: re-schedule on ErrJSONNoSpaceLeft per optimistic concurrency control
	volumeID, err := spdkNode.CreateVolume(lvstore, sizeMiB)
	if err != nil {
		return nil, err
	}

	return &volume{
		name:     req.Name,
		spdkNode: spdkNode,
		csiVolume: csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: sizeMiB * 1024 * 1024,
			VolumeContext: req.GetParameters(),
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func publishVolume(volume *volume) (map[string]string, error) {
	err := volume.spdkNode.PublishVolume(volume.csiVolume.GetVolumeId())
	if err != nil {
		return nil, err
	}

	volumeInfo, err := volume.spdkNode.VolumeInfo(volume.csiVolume.GetVolumeId())
	if err != nil {
		unpublishVolume(volume) // nolint:errcheck // we can do little
		return nil, err
	}
	return volumeInfo, nil
}

func deleteVolume(volume *volume) error {
	return volume.spdkNode.DeleteVolume(volume.csiVolume.GetVolumeId())
}

func unpublishVolume(volume *volume) error {
	return volume.spdkNode.UnpublishVolume(volume.csiVolume.GetVolumeId())
}

// simplest volume scheduler: find first node:lvstore with enough free space
func (cs *controllerServer) schedule(sizeMiB int64) (spdkNode util.SpdkNode, lvstore string, err error) {
	for _, spdkNode := range cs.spdkNodes {
		// retrieve lastest lvstore info from spdk node
		lvstores, err := spdkNode.LvStores()
		if err != nil {
			klog.Errorf("failed to get lvstores from node %s: %s", spdkNode.Info(), err.Error())
			continue
		}
		// check if lvstore has enough free space
		for i := range lvstores {
			lvstore := &lvstores[i]
			if lvstore.FreeSizeMiB > sizeMiB {
				return spdkNode, lvstore.Name, nil
			}
		}
		klog.Infof("not enough free space from node %s", spdkNode.Info())
	}

	return nil, "", fmt.Errorf("failed to find node with enough free space")
}

func newControllerServer(d *csicommon.CSIDriver) (*controllerServer, error) {
	server := controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		volumes:                 make(map[string]*volume),
		volumesIdem:             make(map[string]string),
	}

	// get spdk node configs, see deploy/kubernetes/config-map.yaml
	var config struct {
		Nodes []struct {
			Name       string `json:"name"`
			URL        string `json:"rpcURL"`
			TargetType string `json:"targetType"`
			TargetAddr string `json:"targetAddr"`
		} `json:"Nodes"`
	}
	configFile := util.FromEnv("SPDKCSI_CONFIG", "/etc/spdkcsi-config/config.json")
	err := util.ParseJSONFile(configFile, &config)
	if err != nil {
		return nil, err
	}

	// get spdk node secrets, see deploy/kubernetes/secret.yaml
	var secret struct {
		Tokens []struct {
			Name     string `json:"name"`
			UserName string `json:"username"`
			Password string `json:"password"`
		} `json:"rpcTokens"`
	}
	secretFile := util.FromEnv("SPDKCSI_SECRET", "/etc/spdkcsi-secret/secret.json")
	err = util.ParseJSONFile(secretFile, &secret)
	if err != nil {
		return nil, err
	}

	// create spdk nodes
	for i := range config.Nodes {
		node := &config.Nodes[i]
		tokenFound := false
		// find secret per node
		for j := range secret.Tokens {
			token := &secret.Tokens[j]
			if token.Name == node.Name {
				tokenFound = true
				spdkNode, err := util.NewSpdkNode(node.URL, token.UserName, token.Password, node.TargetType, node.TargetAddr)
				if err != nil {
					klog.Errorf("failed to create spdk node %s: %s", node.Name, err.Error())
				} else {
					klog.Infof("spdk node created: name=%s, url=%s", node.Name, node.URL)
					server.spdkNodes = append(server.spdkNodes, spdkNode)
				}
				break
			}
		}
		if !tokenFound {
			klog.Errorf("failed to find secret for spdk node %s", node.Name)
		}
	}
	if len(server.spdkNodes) == 0 {
		return nil, fmt.Errorf("no valid spdk node found")
	}

	return &server, nil
}
