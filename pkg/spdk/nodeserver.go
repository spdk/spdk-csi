/*
Copyright 2020 The SPDK-CSI Authors.

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
	"os"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"

	csicommon "github.com/spdk/spdk-csi/pkg/csi-common"
	"github.com/spdk/spdk-csi/pkg/util"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter mount.Interface
	volumes map[string]*nodeVolume
	mtx     sync.Mutex // protect volumes map
}

type nodeVolume struct {
	initiator   util.SpdkCsiInitiator
	stagingPath string
	tryLock     util.TryLock
}

func newNodeServer(d *csicommon.CSIDriver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
		mounter:           mount.New(""),
		volumes:           make(map[string]*nodeVolume),
	}
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	volume, err := func() (*nodeVolume, error) {
		volumeID := req.GetVolumeId()
		ns.mtx.Lock()
		defer ns.mtx.Unlock()

		volume, exists := ns.volumes[volumeID]
		if !exists {
			initiator, err := util.NewSpdkCsiInitiator(req.GetVolumeContext())
			if err != nil {
				return nil, err
			}
			volume = &nodeVolume{
				initiator:   initiator,
				stagingPath: "",
			}
			ns.volumes[volumeID] = volume
		}
		return volume, nil
	}()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if volume.tryLock.Lock() {
		defer volume.tryLock.Unlock()

		if volume.stagingPath != "" {
			klog.Warning("volume already staged")
			return &csi.NodeStageVolumeResponse{}, nil
		}
		devicePath, err := volume.initiator.Connect() // idempotent
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		stagingPath, err := ns.stageVolume(devicePath, req) // idempotent
		if err != nil {
			volume.initiator.Disconnect() // nolint:errcheck // ignore error
			return nil, status.Error(codes.Internal, err.Error())
		}
		volume.stagingPath = stagingPath
		return &csi.NodeStageVolumeResponse{}, nil
	}
	return nil, status.Error(codes.Aborted, "concurrent request ongoing")
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	ns.mtx.Lock()
	volume, exists := ns.volumes[volumeID]
	ns.mtx.Unlock()
	if !exists {
		return nil, status.Error(codes.NotFound, volumeID)
	}

	err := func() error {
		if volume.tryLock.Lock() {
			defer volume.tryLock.Unlock()

			if volume.stagingPath == "" {
				klog.Warning("volume already unstaged")
				return nil
			}
			err := ns.deleteMountPoint(volume.stagingPath) // idempotent
			if err != nil {
				return status.Errorf(codes.Internal, "unstage volume %s failed: %s", volumeID, err)
			}
			err = volume.initiator.Disconnect() // idempotent
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}
			volume.stagingPath = ""
			return nil
		}
		return status.Error(codes.Aborted, "concurrent request ongoing")
	}()
	if err != nil {
		return nil, err
	}

	ns.mtx.Lock()
	delete(ns.volumes, volumeID)
	ns.mtx.Unlock()
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	ns.mtx.Lock()
	volume, exists := ns.volumes[volumeID]
	ns.mtx.Unlock()
	if !exists {
		return nil, status.Error(codes.NotFound, volumeID)
	}

	if volume.tryLock.Lock() {
		defer volume.tryLock.Unlock()

		if volume.stagingPath == "" {
			return nil, status.Error(codes.Aborted, "volume unstaged")
		}
		err := ns.publishVolume(volume.stagingPath, req) // idempotent
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.NodePublishVolumeResponse{}, nil
	}
	return nil, status.Error(codes.Aborted, "concurrent request ongoing")
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	ns.mtx.Lock()
	volume, exists := ns.volumes[volumeID]
	ns.mtx.Unlock()
	if !exists {
		return nil, status.Error(codes.NotFound, volumeID)
	}

	if volume.tryLock.Lock() {
		defer volume.tryLock.Unlock()

		err := ns.deleteMountPoint(req.GetTargetPath()) // idempotent
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}
	return nil, status.Error(codes.Aborted, "concurrent request ongoing")
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}

// must be idempotent
func (ns *nodeServer) stageVolume(devicePath string, req *csi.NodeStageVolumeRequest) (string, error) {
	stagingPath := req.GetStagingTargetPath() + "/" + req.GetVolumeId()
	mounted, err := ns.createMountPoint(stagingPath)
	if err != nil {
		return "", err
	}
	if mounted {
		return stagingPath, nil
	}

	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mntFlags := req.GetVolumeCapability().GetMount().GetMountFlags()

	switch req.VolumeCapability.AccessMode.Mode {
	case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY:
		mntFlags = append(mntFlags, "ro")
	case csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
		return "", errors.New("unsupport MULTI_NODE_MULTI_WRITER AccessMode")
	}

	klog.Infof("mount %s to %s, fstype: %s, flags: %v", devicePath, stagingPath, fsType, mntFlags)
	mounter := mount.SafeFormatAndMount{Interface: ns.mounter, Exec: exec.New()}
	err = mounter.FormatAndMount(devicePath, stagingPath, fsType, mntFlags)
	if err != nil {
		return "", err
	}
	return stagingPath, nil
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
		err = os.MkdirAll(path, 0755)
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
