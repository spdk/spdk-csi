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
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	smarpc "github.com/spdk/sma-goapi/v1alpha1"
	"github.com/spdk/sma-goapi/v1alpha1/nvme"
	"github.com/spdk/sma-goapi/v1alpha1/nvmf"
	"github.com/spdk/sma-goapi/v1alpha1/nvmf_tcp"
	"k8s.io/klog"
)

const (
	smaNvmfTCPTargetType = "tcp"
	smaNvmfTCPAdrFam     = "ipv4"
	smaNvmfTCPTargetAddr = "127.0.0.1"
	smaNvmfTCPTargetPort = "4421"
	smaNvmfTCPSubNqnPref = "nqn.2022-04.io.spdk.csi:cnode0:uuid:"
)

func NewSpdkCsiSmaInitiator(volumeContext map[string]string, smaClient smarpc.StorageManagementAgentClient, smaTargetType string, kvmPciBridges int) (SpdkCsiInitiator, error) {
	iSmaCommon := &smaCommon{
		smaClient:     smaClient,
		volumeContext: volumeContext,
		timeout:       60 * time.Second,
	}
	switch smaTargetType {
	case "xpu-sma-nvmftcp":
		return &smainitiatorNvmfTCP{sma: iSmaCommon}, nil
	case "xpu-sma-nvme":
		return &smainitiatorNvme{
			sma:           iSmaCommon,
			kvmPciBridges: kvmPciBridges,
		}, nil
	default:
		return nil, fmt.Errorf("unknown SMA targetType: %s", smaTargetType)
	}
}

// FIXME (JingYan): deviceHandle will be empty after restarting nodeserver, which will cause Disconnect() function to fail.
// So deviceHandle should be persistent, one way to solve it is storing deviceHandle in the file "volume-context.json",
// once the patch https://review.spdk.io/gerrit/c/spdk/spdk-csi/+/16237 gets merged.

type smaCommon struct {
	smaClient     smarpc.StorageManagementAgentClient
	deviceHandle  string
	volumeContext map[string]string
	timeout       time.Duration
	volumeID      []byte
}

// phyIDLock used to synchronize physical function usage
// between concurrent Connect{Nmve,VirtioBlk}() calls.
var phyIDLock sync.Mutex

type smainitiatorNvmfTCP struct {
	sma *smaCommon
}

type smainitiatorNvme struct {
	sma           *smaCommon
	kvmPciBridges int
}

func (sma *smaCommon) ctxTimeout() (context.Context, context.CancelFunc) {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), sma.timeout)
	return ctxTimeout, cancel
}

func (sma *smaCommon) CreateDevice(client smarpc.StorageManagementAgentClient, req *smarpc.CreateDeviceRequest) error {
	ctxTimeout, cancel := sma.ctxTimeout()
	defer cancel()

	klog.Infof("SMA.CreateDevice(%s) = ...", req)
	response, err := client.CreateDevice(ctxTimeout, req)
	if err != nil {
		return fmt.Errorf("SMA.CreateDevice(%s) error: %w", req, err)
	}
	klog.Infof("SMA.CreateDevice(...) => %+v", response)

	if response == nil {
		return fmt.Errorf("SMA.CreateDevice(%s) error: nil response", req)
	}
	if response.Handle == "" {
		return fmt.Errorf("SMA.CreateDevice(%s) error: no device handle in response", req)
	}
	sma.deviceHandle = response.Handle

	return nil
}

func (sma *smaCommon) AttachVolume(client smarpc.StorageManagementAgentClient, req *smarpc.AttachVolumeRequest) error {
	ctxTimeout, cancel := sma.ctxTimeout()
	defer cancel()

	klog.Infof("SMA.AttachVolume(%s) = ...", req)
	response, err := client.AttachVolume(ctxTimeout, req)
	if err != nil {
		return fmt.Errorf("SMA.AttachVolume(%s) error: %w", req, err)
	}
	klog.Infof("SMA.AttachVolume(...) <= %+v", response)

	if response == nil {
		return fmt.Errorf("SMA.AttachVolume(%s) error: nil response", req)
	}

	return nil
}

func (sma *smaCommon) DetachVolume(client smarpc.StorageManagementAgentClient, req *smarpc.DetachVolumeRequest) error {
	ctxTimeout, cancel := sma.ctxTimeout()
	defer cancel()

	klog.Infof("SMA.DetachVolume(%s) = ...", req)
	response, err := client.DetachVolume(ctxTimeout, req)
	if err != nil {
		return fmt.Errorf("SMA.DetachVolume(%s) error: %w", req, err)
	}
	klog.Infof("SMA.DetachVolume(...) => %+v", response)

	if response == nil {
		return fmt.Errorf("SMA.DetachVolume(%s) error: nil response", req)
	}

	return nil
}

func (sma *smaCommon) DeleteDevice(client smarpc.StorageManagementAgentClient, req *smarpc.DeleteDeviceRequest) error {
	ctxTimeout, cancel := sma.ctxTimeout()
	defer cancel()

	klog.Infof("SMA.DeleteDevice(%s) = ...", req)
	response, err := client.DeleteDevice(ctxTimeout, req)
	if err != nil {
		return fmt.Errorf("SMA.DeleteDevice(%s) error: %w", req, err)
	}
	klog.Infof("SMA.DeleteDevice(...) => %+v", response)

	if response == nil {
		return fmt.Errorf("SMA.DeleteDevice(%s) error: nil response", req)
	}
	sma.deviceHandle = ""

	return nil
}

func (sma *smaCommon) volumeUUID() error {
	if sma.volumeContext["model"] == "" {
		return fmt.Errorf("no volume available")
	}

	volUUID, err := uuid.Parse(sma.volumeContext["model"])
	if err != nil {
		return fmt.Errorf("uuid.Parse(%s) failed: %w", sma.volumeContext["model"], err)
	}

	volUUIDBytes, err := volUUID.MarshalBinary()
	if err != nil {
		return fmt.Errorf("%+v MarshalBinary() failed: %w", volUUID, err)
	}

	sma.volumeID = volUUIDBytes
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

// re-use the Connect() and Disconnect() functions from initiator.go
func (i *smainitiatorNvmfTCP) initiatorNVMf() *initiatorNVMf {
	return &initiatorNVMf{
		targetType: smaNvmfTCPTargetType,
		targetAddr: smaNvmfTCPTargetAddr,
		targetPort: smaNvmfTCPTargetPort,
		nqn:        smaNvmfTCPSubNqnPref + i.sma.volumeContext["model"],
		model:      i.sma.volumeContext["model"],
	}
}

// Note: SMA NvmfTCP is not really meant to be used in production, it's mostly there to demonstrate how different device types can be implemented in SMA.
// For SMA NvmfTCP Connect(), three steps will be included,
// - Creates a new device, which is an entity that can be used to expose volumes (e.g. an NVMeoF subsystem).
//   NVMe/TCP parameters will be needed for CreateDeviceRequest, here, the local IP address (A), port (B), subsystem (C) will be specified in the NVMe/TCP parameters.
//   IP will be 127.0.0.1, port will be 4421, and subsystem will be a fixed prefix "nqn.2022-04.io.spdk.csi:cnode0:uuid:" plus the volume uuid.
// - Attach a volume to a specified device will make this volume available through that device.
// - Once AttachVolume succeeds, "nvme connect" will initiate target connection and returns local block device filename.
//   e.g., /dev/disk/by-id/nvme-uuid.d7286022-fe99-422a-b5ce-1295382c2969
// If CreateDevice succeeds, while AttachVolume fails, will call DeleteDevice to clean up
// If CreateDevice and AttachVolume succeed, while "nvme connect" fails, will call Disconnect() to clean up

func (i *smainitiatorNvmfTCP) Connect() (string, error) {
	if err := i.sma.volumeUUID(); err != nil {
		return "", err
	}

	// CreateDevice for SMA NvmfTCP
	createReq := &smarpc.CreateDeviceRequest{
		Volume: nil,
		Params: &smarpc.CreateDeviceRequest_NvmfTcp{
			NvmfTcp: &nvmf_tcp.DeviceParameters{
				Subnqn:       smaNvmfTCPSubNqnPref + i.sma.volumeContext["model"],
				Adrfam:       smaNvmfTCPAdrFam,
				Traddr:       smaNvmfTCPTargetAddr,
				Trsvcid:      smaNvmfTCPTargetPort,
				AllowAnyHost: true,
			},
		},
	}
	if err := i.sma.CreateDevice(i.sma.smaClient, createReq); err != nil {
		return "", err
	}

	// AttachVolume for SMA NvmfTCP
	attachReq := &smarpc.AttachVolumeRequest{
		Volume: &smarpc.VolumeParameters{
			VolumeId:         i.sma.volumeID,
			ConnectionParams: i.sma.nvmfVolumeParameters(),
		},
		DeviceHandle: i.sma.deviceHandle,
	}
	if err := i.sma.AttachVolume(i.sma.smaClient, attachReq); err != nil {
		// Call DeleteDevice to clean up if AttachVolume failed, while CreateDevice succeeded
		klog.Errorf("SMA.NvmfTCP calling DeleteDevice to clean up as AttachVolume error: %s", err)
		deleteReq := &smarpc.DeleteDeviceRequest{
			Handle: i.sma.deviceHandle,
		}
		if errx := i.sma.DeleteDevice(i.sma.smaClient, deleteReq); errx != nil {
			klog.Errorf("SMA.NvmfTCP calling DeleteDevice to clean up error: %s", errx)
		}
		return "", err
	}

	// Initiate target connection with cmd, nvme connect -t tcp -a "127.0.0.1" -s 4421 -n "nqn.2022-04.io.spdk.csi:cnode0:uuid:*"
	devicePath, err := i.initiatorNVMf().Connect()
	if err != nil {
		// Call Disconnect(), including DetachVolume and DeleteDevice, to clean up if nvme connect failed, while CreateDevice and AttachVolume succeeded
		klog.Errorf("SMA.NvmfTCP calling DetachVolume and DeleteDevice to clean up as nvme connect command error: %s", err)
		if errx := i.Disconnect(); errx != nil {
			klog.Errorf("SMA.NvmfTCP calling DetachVolume and DeleteDevice to clean up error: %s", errx)
		}
		return "", err
	}

	return devicePath, nil
}

// For SMA NvmfTCP Disconnect(), "nvme disconnect" will be executed first to terminate the target connection,
// then, DetachVolume() will be called to detache the volume from the device,
// finally, DeleteDevice() will help to delete the device created in the Connect() function.
// If "nvme disconnect" fails, will continue DetachVolume and DeleteDevice to clean up
// If DetachVolume, will continue DeleteDevice to clean up

func (i *smainitiatorNvmfTCP) Disconnect() error {
	// nvme disconnect -n "nqn.2022-04.io.spdk.csi:cnode0:uuid:*"
	if err := i.initiatorNVMf().Disconnect(); err != nil {
		// go on checking device status in case caused by duplicate request
		klog.Errorf("SMA.NvmfTCP nvme disconnect command error: %s", err)
	}

	// DetachVolume for SMA NvmfTCP
	detachReq := &smarpc.DetachVolumeRequest{
		VolumeId:     i.sma.volumeID,
		DeviceHandle: i.sma.deviceHandle,
	}
	if err := i.sma.DetachVolume(i.sma.smaClient, detachReq); err != nil {
		klog.Errorf("SMA.NvmfTCP DetachVolume error: %s", err)
	}

	// DeleteDevice for SMA NvmfTCP
	deleteReq := &smarpc.DeleteDeviceRequest{
		Handle: i.sma.deviceHandle,
	}
	if err := i.sma.DeleteDevice(i.sma.smaClient, deleteReq); err != nil {
		klog.Errorf("SMA.NvmfTCP DeleteDevice error: %s", err)
		return err
	}
	return nil
}

func (i *smainitiatorNvme) smainitiatorNvmeCleanup() {
	detachReq := &smarpc.DetachVolumeRequest{
		VolumeId:     i.sma.volumeID,
		DeviceHandle: i.sma.deviceHandle,
	}
	if err := i.sma.DetachVolume(i.sma.smaClient, detachReq); err != nil {
		klog.Errorf("SMA.Nvme calling DetachVolume to clean up error: %s", err)
	}
}

// For SMA Nvme Connect(), CreateDevice and AttachVolume will be included.
// - Creates a new device if the deviceHandle is empty, which is an entity that can be used to expose volumes (e.g. an NVMeoF subsystem).
//   PhysicalId and VirtualId information in SMA Nvme CreateDeviceRequest is supposed to be set according to the xPU hardware.
//   As we are using KVM case now, in  "deploy/spdk/sma.yaml", the name, buses and count of pci-bridge are configured for vfiouser when starting sma server.
//   Generally, when using KVM, the VirtualId is always 0, and the range of PhysicalId is from 0 to the sum of buses-counts (namely 64 in our case).
// - Attach a volume to a specified device will make this volume available through that device.
//   Once AttachVolume succeeds, a local block device will be available, e.g., /dev/nvme0n1

func (i *smainitiatorNvme) Connect() (string, error) {
	if err := i.sma.volumeUUID(); err != nil {
		return "", err
	}

	// CreateDevice for SMA Nvme
	devicePath, err := CheckIfNvmeDeviceExists(i.sma.volumeContext["model"], nil)
	if devicePath != "" {
		klog.Infof("Found existing device for '%s': %v", i.sma.volumeContext["mode"], devicePath)
		return devicePath, nil
	}
	if !os.IsNotExist(err) {
		klog.Errorf("failed to detect existing nvme device for '%s'", i.sma.volumeContext["model"])
	}
	phyIDLock.Lock()
	defer phyIDLock.Unlock()
	pf, vf, err := GetNvmeAvailableFunction(i.kvmPciBridges)
	if err != nil {
		return "", fmt.Errorf("failed to detect free NVMe virtual function: %w", err)
	}
	// SMA always expects Vf value as 0. It detects the right KVM bus from Pf value.
	vPf := pf*32 + vf
	klog.Infof("Using next available function: %d", vf)
	createReq := &smarpc.CreateDeviceRequest{
		Params: &smarpc.CreateDeviceRequest_Nvme{
			Nvme: &nvme.DeviceParameters{
				PhysicalId: vPf,
				VirtualId:  0,
			},
		},
	}
	if err = i.sma.CreateDevice(i.sma.smaClient, createReq); err != nil {
		klog.Errorf("SMA.Nvme CreateDevice error: %s", err)
		return "", err
	}

	// AttachVolume for SMA Nvme
	attachReq := &smarpc.AttachVolumeRequest{
		Volume: &smarpc.VolumeParameters{
			VolumeId:         i.sma.volumeID,
			ConnectionParams: i.sma.nvmfVolumeParameters(),
		},
		DeviceHandle: i.sma.deviceHandle,
	}
	if err = i.sma.AttachVolume(i.sma.smaClient, attachReq); err != nil {
		klog.Errorf("SMA.Nvme AttachVolume error: %s", err)
		if delErr := i.sma.DeleteDevice(i.sma.smaClient, &smarpc.DeleteDeviceRequest{
			Handle: i.sma.deviceHandle,
		}); delErr != nil {
			klog.Errorf("Failed to remove device '%s': %v. Review and clean it manually!", delErr, i.sma.deviceHandle)
		}
		return "", err
	}

	bdf := fmt.Sprintf("0000:%02x:%02x.0", pf+1, vf)
	klog.Infof("Waiting still device ready for '%s' at '%s' ...", i.sma.volumeContext["model"], bdf)
	devicePath, err = GetNvmeDeviceName(i.sma.volumeContext["model"], bdf)
	if err != nil {
		klog.Errorf("Could not detect the device: %s", err)
		i.smainitiatorNvmeCleanup()
		return "", err
	}

	return devicePath, nil
}

// For SMA Nvme Disconnect(), two steps will be included, namely DetachVolume and DeleteDevice.

func (i *smainitiatorNvme) Disconnect() error {
	// DetachVolume for SMA Nvme
	devicePath, err := CheckIfNvmeDeviceExists(i.sma.volumeContext["model"], nil)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if devicePath == "" {
		return nil
	}
	detachReq := &smarpc.DetachVolumeRequest{
		VolumeId:     i.sma.volumeID,
		DeviceHandle: i.sma.deviceHandle,
	}
	if err := i.sma.DetachVolume(i.sma.smaClient, detachReq); err != nil {
		klog.Errorf("SMA.Nvme DetachVolume error: %s", err)
		return err
	}

	// DeleteDevice for SMA Nvme
	if err := i.sma.DeleteDevice(i.sma.smaClient, &smarpc.DeleteDeviceRequest{
		Handle: i.sma.deviceHandle,
	}); err != nil {
		klog.Errorf("SMA.Nvme DeleteDevice error: %s", err)
		return err
	}

	return waitForDeviceGone(devicePath)
}
