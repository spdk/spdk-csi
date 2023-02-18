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
	"time"

	"github.com/google/uuid"
	smarpc "github.com/spdk/sma-goapi/v1alpha1"
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

func NewSpdkCsiSmaInitiator(volumeContext map[string]string, smaClient smarpc.StorageManagementAgentClient, smaTargetType string) (SpdkCsiInitiator, error) {
	iSmaCommon := &smaCommon{
		smaClient:     smaClient,
		volumeContext: volumeContext,
		timeout:       60 * time.Second,
	}
	switch smaTargetType {
	case "xpu-sma-nvmftcp":
		return &smainitiatorNvmfTCP{sma: iSmaCommon}, nil
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

type smainitiatorNvmfTCP struct {
	sma *smaCommon
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
	klog.Infof("SMA.AttachVolume(...) => %+v", response)

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
