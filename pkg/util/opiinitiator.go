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
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"

	"google.golang.org/grpc"
	"k8s.io/klog"

	opiapiStorage "github.com/opiproject/opi-api/storage/v1alpha1/gen/go"
)

const (
	opiNvmfRemoteControllerHostnqnPref = "nqn.2023-04.io.spdk.csi:remote.controller:uuid:"
	opiNvmeSubsystemNqnPref            = "nqn.2016-06.io.spdk.csi:subsystem:uuid:"
	opiObjectPrefix                    = "opi-spckcsi-"
)

type opiCommon struct {
	opiClient                  *grpc.ClientConn
	volumeContext              map[string]string
	nvmfRemoteControllerClient opiapiStorage.NvmeRemoteControllerServiceClient
	nvmfRemoteControllerName   string
	nvmfPathName               string
}

type opiInitiatorNvme struct {
	*opiCommon
	frontendNvmeClient opiapiStorage.FrontendNvmeServiceClient
	subsystemName      string
	namespaceName      string
	nvmeControllerName string
}

var _ XpuInitiator = &opiInitiatorNvme{}

type opiInitiatorVirtioBlk struct {
	*opiCommon
	frontendVirtioBlkClient opiapiStorage.FrontendVirtioBlkServiceClient
	virtioBlkName           string
}

var _ XpuInitiator = &opiInitiatorVirtioBlk{}

func NewSpdkCsiOpiInitiator(volumeContext map[string]string, xpuClient *grpc.ClientConn, trType TransportType, storedXPUContext map[string]string) (XpuInitiator, error) {
	iOpiCommon := &opiCommon{
		opiClient:                  xpuClient,
		volumeContext:              volumeContext,
		nvmfRemoteControllerClient: opiapiStorage.NewNvmeRemoteControllerServiceClient(xpuClient),
	}
	switch trType {
	case TransportTypeVirtioBlk:
		iOpiInitiatorVirtioBlk := &opiInitiatorVirtioBlk{opiCommon: iOpiCommon}
		if storedXPUContext != nil {
			iOpiCommon.nvmfRemoteControllerName = storedXPUContext["nvmfRemoteControllerName"]
			iOpiCommon.nvmfPathName = storedXPUContext["nvmfPathName"]
			iOpiInitiatorVirtioBlk.virtioBlkName = storedXPUContext["virtioBlkName"]
		}
		return &opiInitiatorVirtioBlk{
			opiCommon:               iOpiCommon,
			frontendVirtioBlkClient: opiapiStorage.NewFrontendVirtioBlkServiceClient(xpuClient),
			virtioBlkName:           iOpiInitiatorVirtioBlk.virtioBlkName,
		}, nil
	case TransportTypeNvme:
		iOpiInitiatorNvme := &opiInitiatorNvme{opiCommon: iOpiCommon}
		if storedXPUContext != nil {
			iOpiCommon.nvmfRemoteControllerName = storedXPUContext["nvmfRemoteControllerName"]
			iOpiCommon.nvmfPathName = storedXPUContext["nvmfPathName"]
			iOpiInitiatorNvme.subsystemName = storedXPUContext["subsystemName"]
			iOpiInitiatorNvme.namespaceName = storedXPUContext["namespaceName"]
			iOpiInitiatorNvme.nvmeControllerName = storedXPUContext["nvmeControllerName"]
		}
		return &opiInitiatorNvme{
			opiCommon:          iOpiCommon,
			frontendNvmeClient: opiapiStorage.NewFrontendNvmeServiceClient(xpuClient),
			subsystemName:      iOpiInitiatorNvme.subsystemName,
			namespaceName:      iOpiInitiatorNvme.namespaceName,
			nvmeControllerName: iOpiInitiatorNvme.nvmeControllerName,
		}, nil
	default:
		return nil, fmt.Errorf("unknown OPI transport type: %s", trType)
	}
}

func (i *opiInitiatorNvme) GetParam() map[string]string {
	opiNvmeContext := make(map[string]string)
	opiNvmeContext["nvmfRemoteControllerName"] = i.opiCommon.nvmfRemoteControllerName
	opiNvmeContext["nvmfPathName"] = i.opiCommon.nvmfPathName
	opiNvmeContext["subsystemName"] = i.subsystemName
	opiNvmeContext["namespaceName"] = i.namespaceName
	opiNvmeContext["nvmeControllerName"] = i.nvmeControllerName
	return opiNvmeContext
}

func (i *opiInitiatorVirtioBlk) GetParam() map[string]string {
	opiVirtioBlkContext := make(map[string]string)
	opiVirtioBlkContext["nvmfRemoteControllerName"] = i.opiCommon.nvmfRemoteControllerName
	opiVirtioBlkContext["nvmfPathName"] = i.opiCommon.nvmfPathName
	opiVirtioBlkContext["virtioBlkName"] = i.virtioBlkName
	return opiVirtioBlkContext
}

// Connect to remote controller, which is needed by OPI VirtioBlk and Nvme
func (opi *opiCommon) createNvmeRemoteController(ctx context.Context) error {
	nvmfRemoteControllerID := opiObjectPrefix + opi.volumeContext["model"]

	createReq := &opiapiStorage.CreateNvmeRemoteControllerRequest{
		NvmeRemoteController: &opiapiStorage.NvmeRemoteController{
			Multipath: opiapiStorage.NvmeMultipath_NVME_MULTIPATH_MULTIPATH,
		},
		NvmeRemoteControllerId: nvmfRemoteControllerID,
	}

	klog.Info("OPI.CreateNvmeRemoteController() => ", createReq)
	createResp, err := opi.nvmfRemoteControllerClient.CreateNvmeRemoteController(ctx, createReq)
	if err != nil {
		return fmt.Errorf("failed to create remote NVMf controller for '%s': %w", nvmfRemoteControllerID, err)
	}
	klog.Info("OPI.CreateNvmeRemoteController() <= ", createResp)
	opi.nvmfRemoteControllerName = createResp.Name
	return nil
}

// Disconnect from remote controller, which is needed by both OPI VirtioBlk and Nvme
func (opi *opiCommon) deleteNvmeRemoteController(ctx context.Context) error {
	klog.Infof("OPI.DeleteNvmeRemoteController with opi.nvmfRemoteControllerName '%s'", opi.nvmfRemoteControllerName)
	if opi.nvmfRemoteControllerName == "" {
		return nil
	}

	// DeleteNvmeRemoteController with "AllowMissing: true", deleting operation will always succeed even the resource is not found
	deleteReq := &opiapiStorage.DeleteNvmeRemoteControllerRequest{
		Name:         opi.nvmfRemoteControllerName,
		AllowMissing: true,
	}
	klog.Info("OPI.DeleteNvmeRemoteController() => ", deleteReq)
	if _, err := opi.nvmfRemoteControllerClient.DeleteNvmeRemoteController(ctx, deleteReq); err != nil {
		klog.Infof("Error on deleting remote NVMf controller '%s': %v", opi.nvmfRemoteControllerName, err)
		return err
	}
	klog.Info("OPI.DeleteNvmeRemoteController successfully")
	opi.nvmfRemoteControllerName = ""

	return nil
}

// Create paths within a controller, which is needed by both OPI VirtioBlk and Nvme
func (opi *opiCommon) createNvmfPath(ctx context.Context) error {
	nvmfPathID := opiObjectPrefix + opi.volumeContext["model"]
	targetSvcPort, err := strconv.ParseInt(opi.volumeContext["targetPort"], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to create NVMf path for '%s': invalid targetPort '%s': %w",
			opi.volumeContext["model"], opi.volumeContext["targetPort"], err)
	}
	createReq := &opiapiStorage.CreateNvmePathRequest{
		NvmePath: &opiapiStorage.NvmePath{
			Trtype:            opiapiStorage.NvmeTransportType_NVME_TRANSPORT_TCP,
			Adrfam:            opiapiStorage.NvmeAddressFamily_NVME_ADRFAM_IPV4,
			Traddr:            opi.volumeContext["targetAddr"],
			Trsvcid:           targetSvcPort,
			Subnqn:            opi.volumeContext["nqn"],
			Hostnqn:           opiNvmfRemoteControllerHostnqnPref + opi.volumeContext["model"],
			ControllerNameRef: opi.nvmfRemoteControllerName,
		},
		NvmePathId: nvmfPathID,
	}

	klog.Info("OPI.CreateNVMfPath() => ", createReq)
	var createResp *opiapiStorage.NvmePath
	createResp, err = opi.nvmfRemoteControllerClient.CreateNvmePath(ctx, createReq)
	if err != nil {
		return fmt.Errorf("failed to create remote NVMf path for '%s': %w", nvmfPathID, err)
	}
	klog.Info("OPI.CreateNVMfPath() <= ", createResp)
	opi.nvmfPathName = createResp.Name
	return nil
}

// Delete paths within a controller, which is needed by both OPI VirtioBlk and Nvme
func (opi *opiCommon) deleteNvmfPath(ctx context.Context) error {
	klog.Infof("OPI.DeleteNVMfPath with opi.nvmfPathName '%s'", opi.nvmfPathName)
	if opi.nvmfPathName == "" {
		return nil
	}

	// DeleteNvmePath with "AllowMissing: true", deleting operation will always succeed even the resource is not found
	deleteReq := &opiapiStorage.DeleteNvmePathRequest{
		Name:         opi.nvmfPathName,
		AllowMissing: true,
	}
	klog.Info("OPI.DeleteNVMfPath() => ", deleteReq)
	if _, err := opi.nvmfRemoteControllerClient.DeleteNvmePath(ctx, deleteReq); err != nil {
		klog.Infof("Error on deleting remote NVMf controller '%s': %v", opi.nvmfPathName, err)
		return err
	}
	klog.Info("OPI.DeleteNVMfPath successfully")
	opi.nvmfPathName = ""

	return nil
}

// Create the Nvme subsystem, which is needed by both OPI VirtioBlk and Nvme
func (i *opiInitiatorNvme) createNvmeSubsystem(ctx context.Context) error {
	nvmeSubsystemID := opiObjectPrefix + i.volumeContext["model"]
	createReq := &opiapiStorage.CreateNvmeSubsystemRequest{
		NvmeSubsystemId: nvmeSubsystemID,
		NvmeSubsystem: &opiapiStorage.NvmeSubsystem{
			Spec: &opiapiStorage.NvmeSubsystemSpec{
				Nqn: opiNvmeSubsystemNqnPref + i.volumeContext["model"],
			},
		},
	}
	klog.Info("OPI.CreateNvmeSubsystem() => ", createReq)
	createResp, err := i.frontendNvmeClient.CreateNvmeSubsystem(ctx, createReq)
	if err != nil {
		return fmt.Errorf("failed to create NVMe subsystem for '%s': %w", nvmeSubsystemID, err)
	}
	klog.Info("OPI.CreateNvmeSubsystem() <= ", createResp)
	i.subsystemName = createResp.Name

	return nil
}

// Delete the Nvme subsystem, which is needed by both OPI VirtioBlk and Nvme
func (i *opiInitiatorNvme) deleteNvmeSubsystem(ctx context.Context) error {
	klog.Infof("OPI.DeleteNvmeSubsystem with i.subsystemName '%s'", i.subsystemName)
	if i.subsystemName == "" {
		return nil
	}

	// DeleteNvmeSubsystem with "AllowMissing: true", deleting operation will always succeed even the resource is not found
	deleteReq := &opiapiStorage.DeleteNvmeSubsystemRequest{
		Name:         i.subsystemName,
		AllowMissing: true,
	}
	klog.Info("OPI.DeleteNvmeSubsystem() => ", deleteReq)
	if _, err := i.frontendNvmeClient.DeleteNvmeSubsystem(ctx, deleteReq); err != nil {
		return fmt.Errorf("failed to delete NVMe subsystem '%s': %w", i.subsystemName, err)
	}
	klog.Info("OPI.DeleteNvmeSubsystem successfully")
	i.subsystemName = ""

	return nil
}

// Create a controller with vfiouser transport information for Nvme
func (i *opiInitiatorNvme) createNvmeController(ctx context.Context, physID uint32) error {
	nvmeControllerID := opiObjectPrefix + i.volumeContext["model"]
	createReq := &opiapiStorage.CreateNvmeControllerRequest{
		NvmeController: &opiapiStorage.NvmeController{
			Spec: &opiapiStorage.NvmeControllerSpec{
				SubsystemNameRef: i.subsystemName,
				PcieId: &opiapiStorage.PciEndpoint{
					PhysicalFunction: int32(physID),
				},
			},
		},
		NvmeControllerId: nvmeControllerID,
	}
	klog.Info("OPI.CreateNvmeController() => ", createReq)
	createResp, err := i.frontendNvmeClient.CreateNvmeController(ctx, createReq)
	if err != nil {
		klog.Errorf("OPI.CreateNvmeController with pfId (%d) error: %s", physID, err)
		return fmt.Errorf("failed to create NVMe controller: %w", err)
	}
	klog.Infof("OPI.CreateNvmeController() with pfId '%d' <= %+v", physID, createResp)
	i.nvmeControllerName = createResp.Name

	return nil
}

// Delete the controller with vfiouser transport information for Nvme
func (i *opiInitiatorNvme) deleteNvmeController(ctx context.Context) (err error) {
	klog.Infof("OPI.DeleteNvmeController with i.nvmeControllerName '%s'", i.nvmeControllerName)
	if i.nvmeControllerName == "" {
		return nil
	}
	// DeleteNvmeController with "AllowMissing: true", deleting operation will always succeed even the resource is not found
	deleteControllerReq := &opiapiStorage.DeleteNvmeControllerRequest{
		Name:         i.nvmeControllerName,
		AllowMissing: true,
	}
	klog.Info("OPI.DeleteNvmeController() => ", deleteControllerReq)
	if _, err = i.frontendNvmeClient.DeleteNvmeController(ctx, deleteControllerReq); err != nil {
		klog.Errorf("OPI.Nvme DeleteNvmeController error: %s", err)
		return err
	}
	klog.Info("OPI.DeleteNvmeController successfully")
	i.nvmeControllerName = ""

	return nil
}

// Get Bdev for the volume and add a new namespace to the subsystem with that bdev for Nvme
func (i *opiInitiatorNvme) createNvmeNamespace(ctx context.Context) error {
	nvmeNamespaceID := opiObjectPrefix + i.volumeContext["model"]
	bigHostNsID, err := rand.Int(rand.Reader, big.NewInt(32))
	if err != nil {
		return fmt.Errorf("failed to generate random number: %w", err)
	}
	createReq := &opiapiStorage.CreateNvmeNamespaceRequest{
		NvmeNamespace: &opiapiStorage.NvmeNamespace{
			Spec: &opiapiStorage.NvmeNamespaceSpec{
				SubsystemNameRef: i.subsystemName,
				HostNsid:         int32(bigHostNsID.Int64()),
				VolumeNameRef:    i.volumeContext["model"],
			},
		},
		NvmeNamespaceId: nvmeNamespaceID,
	}
	klog.Info("OPI.CreateNvmeNamespace() => ", createReq)
	createResp, err := i.frontendNvmeClient.CreateNvmeNamespace(ctx, createReq)
	if err != nil {
		klog.Infof("Failed to create nvme namespace '%s': %v", nvmeNamespaceID, err)
		return err
	}
	klog.Info("OPI.CreateNvmeNamespace() <= ", createResp)
	i.namespaceName = createResp.Name
	return nil
}

// Delete the namespace from the subsystem with the bdev for Nvme
func (i *opiInitiatorNvme) deleteNvmeNamespace(ctx context.Context) error {
	klog.Infof("OPI.DeleteNvmeNamespace with i.namespaceName '%s'", i.namespaceName)
	if i.namespaceName == "" {
		return nil
	}

	// DeleteNvmeNamespace with "AllowMissing: true", deleting operation will always succeed even the resource is not found
	deleteNvmeNamespaceReq := &opiapiStorage.DeleteNvmeNamespaceRequest{
		Name:         i.namespaceName,
		AllowMissing: true,
	}
	klog.Info("OPI.DeleteNvmeNamespace() => ", deleteNvmeNamespaceReq)

	if _, err := i.frontendNvmeClient.DeleteNvmeNamespace(ctx, deleteNvmeNamespaceReq); err != nil {
		klog.Errorf("OPI.Nvme DeleteNvmeNamespace error: %s", err)
		return err
	}
	klog.Info("OPI.DeleteNvmeNamespace successfully")
	i.namespaceName = ""

	return nil
}

// For OPI Nvme Connect(), five steps will be included.
// First, to create a new subsystem, the nqn (nqn.2016-06.io.spdk.csi:subsystem:uuid:VolumeId) will be set in the CreateNvmeSubsystemRequest.
// After a successful CreateNvmeSubsystemRequest, a nvmf subsystem with the nqn will be created on the xPU node.
// Second, create a controller with vfiouser transport information, we are using the KVM case now,
// and the only information needed in the CreateNvmeControllerRequest is pfId, which should be from 0 to the sum of buses-counts (namely 64 in our case).
// After a successful CreateNvmeControllerRequest, the "listen_addresses" field in the nvmf subsystem created in the first step
// will be filled in with VFIOUSER related information, including transport (VFIOUSER), trtype (VFIOUSER), adrfam (IPv4) and traddr (/var/tmp/controller$pfId).
// Third, to connect to the remote controller, this step is used to connect to the storage node.
// Fourth, to create multipath for connection, in this step, paths are then created within a controller.
// Finally, to get Bdev for the volume and add a new namespace to the subsystem with that bdev. After this step, the Nvme block device will appear.
// If any step above fails, call a cleanup operation to clean the resources created previously.
func (i *opiInitiatorNvme) Connect(ctx context.Context, params *ConnectParams) error {
	failed := true
	// step 1: create a subsystem
	if err := i.createNvmeSubsystem(ctx); err != nil {
		return err
	}
	defer func() {
		if failed {
			klog.Info("Cleaning up incomplete OPI Nvme creation...")
			if err := i.cleanup(ctx); err != nil {
				klog.Errorf("cleanup failure: %v", err)
			}
		}
	}()
	// step 2: create a controller with vfiouser transport information
	if err := i.createNvmeController(ctx, params.vPf); err != nil {
		return err
	}
	// step 3: connect to remote controller
	if err := i.createNvmeRemoteController(ctx); err != nil {
		return err
	}
	// step 4: create multipath for connection
	if err := i.createNvmfPath(ctx); err != nil {
		return err
	}
	// step 5: get bdev for the volume and add a new namespace to the subsystem with that bdev
	if err := i.createNvmeNamespace(ctx); err != nil {
		return err
	}
	failed = false

	return nil
}

// For OPI Nvme Disconnect(), all the resources created in the Connect() function will be deleted reversely,
// namely, deleteNvmeNamespace, deleteNvmfPath, deleteNvmeController, deleteNvmeRemoteController, and deleteNvmeSubsystem.
// All these five functions are encrypted in a new cleanup() function, as all the deleting operations have "AllowMissing: true" in the request,
// they will always succeed even if the resources are not found.
func (i *opiInitiatorNvme) Disconnect(ctx context.Context) error {
	return i.cleanup(ctx)
}

func (i *opiInitiatorNvme) cleanup(ctx context.Context) error {
	if err := i.deleteNvmeNamespace(ctx); err != nil {
		return err
	}
	if err := i.deleteNvmfPath(ctx); err != nil {
		return err
	}
	if err := i.deleteNvmeRemoteController(ctx); err != nil {
		return err
	}
	if err := i.deleteNvmeController(ctx); err != nil {
		return err
	}

	return i.deleteNvmeSubsystem(ctx)
}

// Create a controller with VirtioBlk transport information Bdev
func (i *opiInitiatorVirtioBlk) createVirtioBlk(ctx context.Context, physID uint32) error {
	virtioBlkID := opiObjectPrefix + i.volumeContext["model"]
	createReq := &opiapiStorage.CreateVirtioBlkRequest{
		VirtioBlk: &opiapiStorage.VirtioBlk{
			PcieId: &opiapiStorage.PciEndpoint{
				PhysicalFunction: int32(physID),
			},
			VolumeNameRef: i.volumeContext["model"],
		},
		VirtioBlkId: virtioBlkID,
	}
	klog.Info("OPI.CreateVirtioBlk() => ", createReq)
	blkDevice, err := i.frontendVirtioBlkClient.CreateVirtioBlk(ctx, createReq)
	if err != nil {
		return fmt.Errorf("failed to create virtio-blk device with pfId (%d) error: %w", physID, err)
	}
	klog.Info("OPI.CreateVirtioBlk() <= ", blkDevice)
	i.virtioBlkName = blkDevice.Name

	return nil
}

// Delete the controller with VirtioBlk transport information Bdev
func (i *opiInitiatorVirtioBlk) deleteVirtioBlk(ctx context.Context) error {
	klog.Infof("OPI.DeleteVirtioBlk with i.virtioBlkName '%s'", i.virtioBlkName)
	if i.virtioBlkName == "" {
		return nil
	}
	// DeleteVirtioBlk, with "AllowMissing: true", deleting operation will always succeed even the resource is not found
	deleteReq := &opiapiStorage.DeleteVirtioBlkRequest{
		Name:         i.virtioBlkName,
		AllowMissing: true,
	}
	klog.Info("OPI.DeleteVirtioBlk() => ", deleteReq)

	_, err := i.frontendVirtioBlkClient.DeleteVirtioBlk(ctx, deleteReq)
	if err != nil {
		klog.Errorf("OPI.Nvme DeleteVirtioBlk error: %s", err)
		return err
	}
	klog.Info("OPI.DeleteVirtioBlk successfully")
	i.virtioBlkName = ""

	return nil
}

// For OPI VirtioBlk Connect(), three steps will be included.
// First, to connect to the remote controller, this step is used to connect to the storage node.
// Second, to create multipath for connection, in this step, paths are then created within a controller.
// Third, to create a controller with virtio_blk transport information bdev, which is calling vhost_create_blk_controller on xPU node.
// After these three steps, a VirtioBlk device will appear.
// If any step above fails, call a cleanup operation to clean the resources created previously.
func (i *opiInitiatorVirtioBlk) Connect(ctx context.Context, params *ConnectParams) error {
	failed := true
	// step 1: connect to remote controller
	if err := i.createNvmeRemoteController(ctx); err != nil {
		return err
	}
	defer func() {
		if failed {
			klog.Info("Cleaning up incomplete OPI VirtioBlk creation...")
			if err := i.cleanup(ctx); err != nil {
				klog.Errorf("cleanup failure: %v", err)
			}
		}
	}()

	// step 2: create multipath for connection
	if err := i.createNvmfPath(ctx); err != nil {
		return err
	}

	// step 3: Create a controller with virtio_blk transport information bdev
	if err := i.createVirtioBlk(ctx, params.vPf); err != nil {
		return err
	}
	failed = false

	return nil
}

// For OPI VirtioBlk Disconnect(), all the resources created in the Connect() function will be deleted reversely,
// namely, deleteVirtioBlk, deleteNvmfPath, and deleteNvmeRemoteController.
// All these five functions are encrypted in a new cleanup() function, as all the deleting operations have "AllowMissing: true" in the request,
// they will always succeed even if the resources are not found.
func (i *opiInitiatorVirtioBlk) Disconnect(ctx context.Context) error {
	return i.cleanup(ctx)
}

func (i *opiInitiatorVirtioBlk) cleanup(ctx context.Context) error {
	if err := i.deleteVirtioBlk(ctx); err != nil {
		return err
	}
	if err := i.deleteNvmfPath(ctx); err != nil {
		return err
	}
	return i.deleteNvmeRemoteController(ctx)
}
