/*
Copyright (c) Intel Corporation.

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

package util

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	smarpc "github.com/spdk/sma-goapi/v1alpha1"
	"github.com/spdk/sma-goapi/v1alpha1/nvme"
	"github.com/spdk/sma-goapi/v1alpha1/nvmf"
	"github.com/spdk/sma-goapi/v1alpha1/nvmf_tcp"
	"github.com/spdk/sma-goapi/v1alpha1/virtio_blk"
	"google.golang.org/grpc"
	"k8s.io/klog"
)

const (
	smaNvmfTCPTargetType = "tcp"
	smaNvmfTCPAdrFam     = "ipv4"
	smaNvmfTCPTargetAddr = "127.0.0.1"
	smaNvmfTCPTargetPort = "4421"
	smaNvmfTCPSubNqnPref = "nqn.2022-04.io.spdk.csi:cnode0:uuid:"
)

func NewSpdkCsiSmaInitiator(volumeContext map[string]string, xpuClient *grpc.ClientConn, trType TransportType, storedXPUContext map[string]string) (XpuInitiator, error) {
	volumeID, err := volumeUUID(volumeContext["model"])
	if err != nil {
		return nil, err
	}

	iSmaCommon := &smaCommon{
		client:        smarpc.NewStorageManagementAgentClient(xpuClient),
		volumeContext: volumeContext,
		volumeID:      volumeID,
	}
	if storedXPUContext != nil {
		iSmaCommon.deviceHandle = storedXPUContext["deviceHandle"]
	}
	switch trType {
	case TransportTypeNvmfTCP:
		return &smaInitiatorNvmfTCP{smaCommon: iSmaCommon}, nil
	case TransportTypeNvme:
		return &smaInitiatorNvme{smaCommon: iSmaCommon}, nil
	case TransportTypeVirtioBlk:
		return &smaInitiatorVirtioBlk{smaCommon: iSmaCommon}, nil
	default:
		return nil, fmt.Errorf("unknown SMA targetType: %s", trType)
	}
}

type smaCommon struct {
	client        smarpc.StorageManagementAgentClient
	volumeContext map[string]string
	deviceHandle  string
	volumeID      []byte
}

type smaInitiatorNvmfTCP struct {
	*smaCommon
}

var _ XpuInitiator = &smaInitiatorNvmfTCP{}

type smaInitiatorNvme struct {
	*smaCommon
}

var _ XpuInitiator = &smaInitiatorNvme{}

type smaInitiatorVirtioBlk struct {
	*smaCommon
}

var _ XpuInitiator = &smaInitiatorVirtioBlk{}

func (sma *smaCommon) GetParam() map[string]string {
	smaContext := make(map[string]string)
	smaContext["deviceHandle"] = sma.deviceHandle
	return smaContext
}

func (sma *smaCommon) CreateDevice(ctx context.Context, req *smarpc.CreateDeviceRequest) error {
	klog.Info("SMA.CreateDevice() => ", req)
	response, err := sma.client.CreateDevice(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create device for id '%s': %w", sma.volumeContext["model"], err)
	}
	if response == nil {
		return fmt.Errorf("create device: nil response")
	}
	klog.Info("SMA.CreateDevice() <= ", response)

	if response.Handle == "" {
		return fmt.Errorf("create device '%s': no device handle in response", sma.volumeContext["model"])
	}
	sma.deviceHandle = response.Handle
	return nil
}

func (sma *smaCommon) AttachVolume(ctx context.Context, req *smarpc.AttachVolumeRequest) error {
	klog.Info("SMA.AttachVolume() => ", req)
	response, err := sma.client.AttachVolume(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to attach volume %q to device '%s': %w", sma.volumeContext["model"], req.DeviceHandle, err)
	}
	if response == nil {
		return fmt.Errorf("attach volume '%s' for device '%s': nil response", sma.volumeContext["model"], req.DeviceHandle)
	}
	klog.Info("SMA.AttachVolume() <= ", response)
	sma.volumeID = req.Volume.VolumeId

	return nil
}

func (sma *smaCommon) DetachVolume(ctx context.Context, req *smarpc.DetachVolumeRequest) error {
	klog.Info("SMA.DetachVolume() => ", req)
	response, err := sma.client.DetachVolume(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to detach volume %q from device '%s': %w", sma.volumeContext["model"], req.DeviceHandle, err)
	}
	if response == nil {
		return fmt.Errorf("detach volume '%s' from volume '%s': nil response", sma.volumeContext["model"], req.DeviceHandle)
	}
	klog.Info("SMA.DetachVolume() <= ", response)
	sma.volumeID = nil

	return nil
}

func (sma *smaCommon) DeleteDevice(ctx context.Context, req *smarpc.DeleteDeviceRequest) error {
	klog.Info("SMA.DeleteDevice() => ", req)
	response, err := sma.client.DeleteDevice(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete device '%s': %w", req.Handle, err)
	}
	if response == nil {
		return fmt.Errorf("delete device '%s': nil response", req.Handle)
	}
	klog.Info("SMA.DeleteDevice() <= ", response)
	sma.deviceHandle = ""
	return nil
}

func (sma *smaCommon) nvmfVolumeParameters() *smarpc.VolumeParameters_Nvmf {
	vcp := &smarpc.VolumeParameters_Nvmf{
		Nvmf: &nvmf.VolumeConnectionParameters{
			Subnqn:  "",
			Hostnqn: sma.volumeContext["nqn"],
			ConnectionParams: &nvmf.VolumeConnectionParameters_Discovery{
				Discovery: &nvmf.VolumeDiscoveryParameters{
					DiscoveryEndpoints: []*nvmf.Address{
						{
							Trtype:  sma.volumeContext["targetType"],
							Traddr:  sma.volumeContext["targetAddr"],
							Trsvcid: sma.volumeContext["targetPort"],
						},
					},
				},
			},
		},
	}
	return vcp
}

// Note: SMA NvmfTCP is not really meant to be used in production, it's mostly there to demonstrate how different device types can be implemented in SMA.
// For SMA NvmfTCP Connect(), three steps will be included,
//   - Creates a new device, which is an entity that can be used to expose volumes (e.g. an NVMeoF subsystem).
//     NVMe/TCP parameters will be needed for CreateDeviceRequest, here, the local IP address (A), port (B), subsystem (C) will be specified in the NVMe/TCP parameters.
//     IP will be 127.0.0.1, port will be 4421, and subsystem will be a fixed prefix "nqn.2022-04.io.spdk.csi:cnode0:uuid:" plus the volume uuid.
//   - Attach a volume to a specified device will make this volume available through that device.
//   - Once AttachVolume succeeds, "nvme connect" will initiate target connection and returns local block device filename.
//     e.g., /dev/disk/by-id/nvme-uuid.d7286022-fe99-422a-b5ce-1295382c2969
//
// If CreateDevice succeeds, while AttachVolume fails, will call DeleteDevice to clean up
// If CreateDevice and AttachVolume succeed, while "nvme connect" fails, will call Disconnect() to clean up
func (i *smaInitiatorNvmfTCP) Connect(ctx context.Context, params *ConnectParams) error {
	// CreateDevice for SMA NvmfTCP
	createReq := &smarpc.CreateDeviceRequest{
		Volume: nil,
		Params: &smarpc.CreateDeviceRequest_NvmfTcp{
			NvmfTcp: &nvmf_tcp.DeviceParameters{
				Subnqn:       xpuNvmfTCPSubNqnPref + i.volumeContext["model"],
				Adrfam:       xpuNvmfTCPAdrFam,
				Traddr:       xpuNvmfTCPTargetAddr,
				Trsvcid:      params.tcpTargetPort,
				AllowAnyHost: true,
			},
		},
	}
	if err := i.CreateDevice(ctx, createReq); err != nil {
		return err
	}

	// AttachVolume for SMA NvmfTCP
	attachReq := &smarpc.AttachVolumeRequest{
		Volume: &smarpc.VolumeParameters{
			VolumeId:         i.volumeID,
			ConnectionParams: i.nvmfVolumeParameters(),
		},
		DeviceHandle: i.deviceHandle,
	}
	if err := i.AttachVolume(ctx, attachReq); err != nil {
		// Call DeleteDevice to clean up if AttachVolume failed, while CreateDevice succeeded
		klog.Errorf("SMA.NvmfTCP calling DeleteDevice to clean up as AttachVolume error: %s", err)
		deleteReq := &smarpc.DeleteDeviceRequest{
			Handle: i.deviceHandle,
		}
		if errx := i.DeleteDevice(ctx, deleteReq); errx != nil {
			klog.Errorf("SMA.NvmfTCP calling DeleteDevice to clean up error: %s", errx)
		}
		return err
	}

	return nil
}

// For SMA NvmfTCP Disconnect(), DetachVolume() will be called to detach the volume from the device,
// then, DeleteDevice() will help to delete the device created in the Connect() function.
// If "nvme disconnect" fails, will continue DetachVolume and DeleteDevice to clean up
// If DetachVolume, will continue DeleteDevice to clean up

func (i *smaInitiatorNvmfTCP) Disconnect(ctx context.Context) error {
	if i.deviceHandle == "" {
		return nil
	}

	if i.volumeID != nil {
		// DetachVolume for SMA NvmfTCP
		if err := i.DetachVolume(ctx, &smarpc.DetachVolumeRequest{
			VolumeId:     i.volumeID,
			DeviceHandle: i.deviceHandle,
		}); err != nil {
			return fmt.Errorf("failed to detach volume '%s' from device '%s': %w", string(i.volumeID), i.deviceHandle, err)
		}
	}

	// DeleteDevice for SMA NvmfTCP
	return i.DeleteDevice(ctx, &smarpc.DeleteDeviceRequest{Handle: i.deviceHandle})
}

// For SMA Nvme Connect(), CreateDevice and AttachVolume will be included.
//   - Creates a new device if the deviceHandle is empty, which is an entity that can be used to expose volumes (e.g. an NVMeoF subsystem).
//     PhysicalId and VirtualId information in SMA Nvme CreateDeviceRequest is supposed to be set according to the xPU hardware.
//     As we are using KVM case now, in  "deploy/spdk/sma.yaml", the name, buses and count of pci-bridge are configured for vfiouser when starting sma server.
//     Generally, when using KVM, the VirtualId is always 0, and the range of PhysicalId is from 0 to the sum of buses-counts (namely 64 in our case).
//   - Attach a volume to a specified device will make this volume available through that device.
//     Once AttachVolume succeeds, a local block device will be available, e.g., /dev/nvme0n1
func (i *smaInitiatorNvme) Connect(ctx context.Context, params *ConnectParams) error {
	createReq := &smarpc.CreateDeviceRequest{
		Params: &smarpc.CreateDeviceRequest_Nvme{
			Nvme: &nvme.DeviceParameters{
				PhysicalId: params.PciEndpoint.pfID,
				VirtualId:  params.PciEndpoint.vfID,
			},
		},
	}
	if err := i.CreateDevice(ctx, createReq); err != nil {
		klog.Errorf("CreateDevice for SMA NVME with PhysicalId (%d) and VirtualID (%d) error: %s", params.PciEndpoint.pfID, params.PciEndpoint.vfID, err)
		return err
	}

	// AttachVolume for SMA Nvme
	attachReq := &smarpc.AttachVolumeRequest{
		Volume: &smarpc.VolumeParameters{
			VolumeId:         i.volumeID,
			ConnectionParams: i.nvmfVolumeParameters(),
		},
		DeviceHandle: i.deviceHandle,
	}
	if err := i.AttachVolume(ctx, attachReq); err != nil {
		klog.Errorf("SMA.Nvme AttachVolume error: %s", err)
		if delErr := i.DeleteDevice(ctx, &smarpc.DeleteDeviceRequest{
			Handle: i.deviceHandle,
		}); delErr != nil {
			klog.Errorf("Failed to remove device '%s': %v. Review and clean it manually!", delErr, i.deviceHandle)
		}
		return err
	}

	return nil
}

// For SMA Nvme Disconnect(), two steps will be included, namely DetachVolume and DeleteDevice.
func (i *smaInitiatorNvme) Disconnect(ctx context.Context) error {
	if i.deviceHandle == "" {
		return nil
	}

	if i.volumeID != nil {
		detachReq := &smarpc.DetachVolumeRequest{
			VolumeId:     i.volumeID,
			DeviceHandle: i.deviceHandle,
		}
		if err := i.DetachVolume(ctx, detachReq); err != nil {
			klog.Errorf("SMA.Nvme DetachVolume error: %s", err)
			return err
		}
	}

	// DeleteDevice for SMA Nvme
	return i.DeleteDevice(ctx, &smarpc.DeleteDeviceRequest{Handle: i.deviceHandle})
}

// For SMA VirtioBlk Connect(), only CreateDevice is needed, which contains the Volume and PhysicalId/VirtualId info in the request.
// As we are using KVM case now, in  "deploy/spdk/sma.yaml", the name, buses and count of pci-bridge are configured for vhost_blk when starting sma server.
// The sma server will talk with qemu VM, which configured with "-device pci-bridge,chassis_nr=1,id=pci.spdk.0, -device pci-bridge,chassis_nr=2,id=pci.spdk.1".
// Generally, when using KVM, the VirtualId is always 0, and the range of PhysicalId is from 0 to the sum of buses-counts (namely 64 in our case).
// Once CreateDevice succeeds, a VirtioBlk block device will appear.
func (i *smaInitiatorVirtioBlk) Connect(ctx context.Context, params *ConnectParams) error {
	// FixMe (avalluri): Currently there is no appropriate way to know
	// if the virtio block device is already created for this request.
	// Hence depending on the cached 'deviceHandle', but this might not
	// work reliably if the driver node driver restarts.
	if i.deviceHandle != "" {
		klog.Infof("Device is already created for '%s': %s", i.volumeID, i.deviceHandle)
		return nil
	}

	createReq := &smarpc.CreateDeviceRequest{
		Volume: &smarpc.VolumeParameters{
			VolumeId:         i.volumeID,
			ConnectionParams: i.nvmfVolumeParameters(),
		},
		Params: &smarpc.CreateDeviceRequest_VirtioBlk{
			VirtioBlk: &virtio_blk.DeviceParameters{
				PhysicalId: params.PciEndpoint.pfID,
				VirtualId:  params.PciEndpoint.vfID,
			},
		},
	}

	if err := i.CreateDevice(ctx, createReq); err != nil {
		klog.Errorf("CreateDevice for SMA VirtioBlk with PhysicalId (%d) and VirtualID (%d) error: %s", params.PciEndpoint.pfID, params.PciEndpoint.vfID, err)
		return err
	}

	return nil
}

// For SMA VirtioBlk Disconnect(), only DeleteDevice is needed.
func (i *smaInitiatorVirtioBlk) Disconnect(ctx context.Context) error {
	if i.deviceHandle == "" {
		return nil
	}
	// DeleteDevice for VirtioBlk
	return i.DeleteDevice(ctx, &smarpc.DeleteDeviceRequest{Handle: i.deviceHandle})
}

func volumeUUID(model string) ([]byte, error) {
	if model == "" {
		return nil, fmt.Errorf("no volume available")
	}

	volUUID, err := uuid.Parse(model)
	if err != nil {
		return nil, fmt.Errorf("uuid.Parse(%s) failed: %w", model, err)
	}

	return volUUID.MarshalBinary()
}
