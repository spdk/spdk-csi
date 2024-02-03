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
	"os"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"

	csicommon "github.com/spdk/spdk-csi/pkg/csi-common"
	"github.com/spdk/spdk-csi/pkg/util"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter       mount.Interface
	volumeLocks   *util.VolumeLocks
	xpuConnClient *grpc.ClientConn
	xpuTargetType string
	kvmPciBridges int
	spdkNode      *util.NodeNVMf
}

func newNodeServer(d *csicommon.CSIDriver) (*nodeServer, error) {
	ns := &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
		mounter:           mount.New(""),
		volumeLocks:       util.NewVolumeLocks(),
	}

	// get xPU nodes' configs, see deploy/kubernetes/nodeserver-config-map.yaml
	// as spdkcsi-nodeservercm configMap volume is optional when deploying k8s, check nodeserver-config-map.yaml is missing or empty
	spdkcsiNodeServerConfigFile := "/etc/spdkcsi-nodeserver-config/nodeserver-config.json"
	spdkcsiNodeServerConfigFileEnv := "SPDKCSI_CONFIG_NODESERVER"
	configFile := util.FromEnv(spdkcsiNodeServerConfigFileEnv, spdkcsiNodeServerConfigFile)
	_, err := os.Stat(configFile)
	klog.Infof("check whether the configuration file (%s) which is supposed to contain xPU info exists", spdkcsiNodeServerConfigFile)
	if os.IsNotExist(err) {
		klog.Infof("configuration file specified in %s (%s by default) is missing or empty", spdkcsiNodeServerConfigFileEnv, spdkcsiNodeServerConfigFile)
		return ns, nil
	}
	//nolint:tagliatelle // not using json:snake case
	var config struct {
		XPUList []struct {
			Name       string `json:"name"`
			TargetType string `json:"targetType"`
			TargetAddr string `json:"targetAddr"`
		} `json:"xpuList"`
		KvmPciBridges int `json:"kvmPciBridges,omitempty"`
	}

	err = util.ParseJSONFile(configFile, &config)
	if err != nil {
		return nil, fmt.Errorf("error in the configuration file specified in %s (%s by default): %w", spdkcsiNodeServerConfigFileEnv, spdkcsiNodeServerConfigFile, err)
	}
	klog.Infof("obtained xPU info (%v) from configuration file (%s)", config.XPUList, spdkcsiNodeServerConfigFile)

	ns.kvmPciBridges = config.KvmPciBridges
	klog.Infof("obtained KvmPciBridges num (%v) from configuration file (%s)", config.KvmPciBridges, spdkcsiNodeServerConfigFile)

	// try to set up a connection to the first available xPU node in the list via grpc
	// once the connection is built, send pings every 10 seconds if there is no activity
	// FIXME (JingYan): when there are multiple xPU nodes, find a better way to choose one to connect with

	var xpuConnClient *grpc.ClientConn
	var xpuTargetType string

	for i := range config.XPUList {
		if config.XPUList[i].TargetType != "" && config.XPUList[i].TargetAddr != "" {
			klog.Infof("TargetType: %v, TargetAddr: %v.", config.XPUList[i].TargetType, config.XPUList[i].TargetAddr)
			conn, err := grpc.Dial(
				config.XPUList[i].TargetAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
				grpc.WithKeepaliveParams(keepalive.ClientParameters{
					Time:                10 * time.Second,
					Timeout:             1 * time.Second,
					PermitWithoutStream: true,
				}),
				grpc.FailOnNonTempDialError(true),
			)
			if err != nil {
				klog.Errorf("failed to connect to xPU node in: %s, %s", config.XPUList[i].TargetAddr, err)
			} else {
				klog.Infof("connected to xPU node %v with TargetType as %v", config.XPUList[i].TargetAddr, config.XPUList[i].TargetType)
				xpuConnClient = conn
				xpuTargetType = config.XPUList[i].TargetType
				break
			}
		} else {
			klog.Errorf("missing xPU TargetType or TargetAddr in xPUList index %d, skipping this xPU node", i)
		}
	}
	if xpuConnClient == nil && xpuTargetType == "" {
		klog.Infof("failed to connect to any xPU node in the xpuList or xpuList is empty, will continue without xPU node")
	}
	ns.xpuConnClient = xpuConnClient
	ns.xpuTargetType = xpuTargetType

	ns.spdkNode, err = NewsimplyBlockClient()
	if err != nil {
		klog.Error("failed to create simplyblock client", err)
		return nil, err
	}
	return ns, nil
}

func (ns *nodeServer) NodeStageVolume(_ context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	unlock := ns.volumeLocks.Lock(volumeID)
	defer unlock()

	stagingParentPath := req.GetStagingTargetPath() // use this directory to persistently store VolumeContext
	stagingTargetPath := getStagingTargetPath(req)

	isStaged, err := ns.isStaged(stagingTargetPath)
	if err != nil {
		klog.Errorf("failed to check isStaged, targetPath: %s err: %v", stagingTargetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if isStaged {
		klog.Warning("volume already staged")
		return &csi.NodeStageVolumeResponse{}, nil
	}

	var initiator util.SpdkCsiInitiator
	vc := req.GetVolumeContext()

	// if ns.xpuConnClient != nil && ns.xpuTargetType != "" {
	// 	vc["stagingParentPath"] = stagingParentPath
	// 	initiator, err = util.NewSpdkCsiXpuInitiator(vc, ns.xpuConnClient, ns.xpuTargetType, ns.kvmPciBridges)
	// } else {
	// 	initiator, err = util.NewSpdkCsiInitiator(req.GetVolumeContext())
	// }

	vc["stagingParentPath"] = stagingParentPath
	initiator, err = util.NewSpdkCsiInitiator(vc, ns.spdkNode)
	if err != nil {
		klog.Errorf("failed to create spdk initiator, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	devicePath, err := initiator.Connect() // idempotent
	if err != nil {
		klog.Errorf("failed to connect initiator, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			initiator.Disconnect() //nolint:errcheck // ignore error
		}
	}()
	if err = ns.stageVolume(devicePath, stagingTargetPath, req); err != nil { // idempotent
		klog.Errorf("failed to stage volume, volumeID: %s devicePath:%s err: %v", volumeID, devicePath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	// stash VolumeContext to stagingParentPath (useful during Unstage as it has no
	// VolumeContext passed to the RPC as per the CSI spec)
	err = util.StashVolumeContext(req.GetVolumeContext(), stagingParentPath)
	if err != nil {
		klog.Errorf("failed to stash volume context, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(_ context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	unlock := ns.volumeLocks.Lock(volumeID)
	defer unlock()

	stagingParentPath := req.GetStagingTargetPath()
	stagingTargetPath := getStagingTargetPath(req)

	isStaged, err := ns.isStaged(stagingTargetPath)
	if err != nil {
		klog.Errorf("failed to check isStaged, targetPath: %s err: %v", stagingTargetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !isStaged {
		klog.Warning("volume already unstaged")
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	err = ns.deleteMountPoint(stagingTargetPath) // idempotent
	if err != nil {
		klog.Errorf("failed to delete mount point, targetPath: %s err: %v", stagingTargetPath, err)
		return nil, status.Errorf(codes.Internal, "unstage volume %s failed: %s", volumeID, err)
	}

	volumeContext, err := util.LookupVolumeContext(stagingParentPath)
	if err != nil {
		klog.Errorf("failed to lookup volume context, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	var initiator util.SpdkCsiInitiator
	if ns.xpuConnClient != nil && ns.xpuTargetType != "" {
		vc := volumeContext
		vc["stagingParentPath"] = stagingParentPath
		initiator, err = util.NewSpdkCsiXpuInitiator(vc, ns.xpuConnClient, ns.xpuTargetType, ns.kvmPciBridges)
	} else {
		initiator, err = util.NewSpdkCsiInitiator(volumeContext, ns.spdkNode)
	}
	if err != nil {
		klog.Errorf("failed to create spdk initiator, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	err = initiator.Disconnect() // idempotent
	if err != nil {
		klog.Errorf("failed to disconnect initiator, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := util.CleanUpVolumeContext(stagingParentPath); err != nil {
		klog.Errorf("failed to clean up volume context, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodePublishVolume(_ context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	unlock := ns.volumeLocks.Lock(volumeID)
	defer unlock()

	err := ns.publishVolume(getStagingTargetPath(req), req) // idempotent
	if err != nil {
		klog.Errorf("failed to publish volume, volumeID: %s err: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(_ context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	unlock := ns.volumeLocks.Lock(volumeID)
	defer unlock()

	err := ns.deleteMountPoint(req.GetTargetPath()) // idempotent
	if err != nil {
		klog.Errorf("failed to delete mount point, targetPath: %s err: %v", req.GetTargetPath(), err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetCapabilities(_ context.Context, _ *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (ns *nodeServer) NodeExpandVolume(_ context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	klog.Infof("NodeExpandVolume: called with args %+v", *req)
	return &csi.NodeExpandVolumeResponse{}, nil
}

// must be idempotent
//
//nolint:cyclop // many cases in switch increases complexity
func (ns *nodeServer) stageVolume(devicePath, stagingPath string, req *csi.NodeStageVolumeRequest) error {
	mounted, err := ns.createMountPoint(stagingPath)
	if err != nil {
		return err
	}
	if mounted {
		return nil
	}

	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mntFlags := req.GetVolumeCapability().GetMount().GetMountFlags()

	switch req.VolumeCapability.AccessMode.Mode {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY:
		mntFlags = append(mntFlags, "ro")
	case csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
		return errors.New("unsupported MULTI_NODE_MULTI_WRITER AccessMode")
	case csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER:
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER:
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER:
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER:
	case csi.VolumeCapability_AccessMode_UNKNOWN:
	}

	klog.Infof("mount %s to %s, fstype: %s, flags: %v", devicePath, stagingPath, fsType, mntFlags)
	mounter := mount.SafeFormatAndMount{Interface: ns.mounter, Exec: exec.New()}
	err = mounter.FormatAndMount(devicePath, stagingPath, fsType, mntFlags)
	if err != nil {
		return err
	}
	return nil
}

// isStaged if stagingPath is a mount point, it means it is already staged, and vice versa
func (ns *nodeServer) isStaged(stagingPath string) (bool, error) {
	unmounted, err := mount.IsNotMountPoint(ns.mounter, stagingPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		klog.Warningf("check is stage error: %v", err)
		return true, err
	}
	return !unmounted, nil
}

// must be idempotent
func (ns *nodeServer) publishVolume(stagingPath string, req *csi.NodePublishVolumeRequest) error {
	targetPath := req.GetTargetPath()
	mounted, err := ns.createMountPoint(targetPath)
	if err != nil {
		return err
	}
	if mounted {
		return nil
	}

	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mntFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
	mntFlags = append(mntFlags, "bind")
	klog.Infof("mount %s to %s, fstype: %s, flags: %v", stagingPath, targetPath, fsType, mntFlags)
	return ns.mounter.Mount(stagingPath, targetPath, fsType, mntFlags)
}

// create mount point if not exists, return whether already mounted
func (ns *nodeServer) createMountPoint(path string) (bool, error) {
	unmounted, err := mount.IsNotMountPoint(ns.mounter, path)
	if os.IsNotExist(err) {
		unmounted = true
		err = os.MkdirAll(path, 0o755)
	}
	if !unmounted {
		klog.Infof("%s already mounted", path)
	}
	return !unmounted, err
}

// unmount and delete mount point, must be idempotent
func (ns *nodeServer) deleteMountPoint(path string) error {
	unmounted, err := mount.IsNotMountPoint(ns.mounter, path)
	if os.IsNotExist(err) {
		klog.Infof("%s already deleted", path)
		return nil
	}
	if err != nil {
		return err
	}
	if !unmounted {
		err = ns.mounter.Unmount(path)
		if err != nil {
			return err
		}
	}
	return os.RemoveAll(path)
}

func getStagingTargetPath(req interface{}) string {
	switch vr := req.(type) {
	case *csi.NodeStageVolumeRequest:
		return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()
	case *csi.NodeUnstageVolumeRequest:
		return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()
	case *csi.NodePublishVolumeRequest:
		return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()
	}
	return ""
}
