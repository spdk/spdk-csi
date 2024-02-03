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
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"k8s.io/klog"
)

const (
	xpuTargetBackendSma  = "sma"
	xpuTargetBackendOpi  = "opi"
	xpuNvmfTCPTargetType = "tcp"
	xpuNvmfTCPAdrFam     = "ipv4"
	xpuNvmfTCPTargetAddr = "127.0.0.1"
	xpuNvmfTCPTargetPort = "4421"
	xpuNvmfTCPSubNqnPref = "nqn.2022-04.io.spdk.csi:cnode0:uuid:"
)

type TransportType string

const (
	TransportTypeNvmfTCP   = "nvmftcp"
	TransportTypeNvme      = "nvme"
	TransportTypeVirtioBlk = "virtioblk"
	TargetTypeCache        = "cache"
)

type XpuInitiator interface {
	Connect(context.Context, *ConnectParams) error
	Disconnect(context.Context) error
	GetParam() map[string]string
}

type ConnectParams struct {
	tcpTargetPort string // NvmfTCP
	vPf           uint32 // VirtiioBlk, and Vfiouser
}

type XpuTargetType struct {
	Backend string
	TrType  TransportType
}

type xpuInitiator struct {
	backend       XpuInitiator
	targetInfo    *XpuTargetType
	volumeContext map[string]string
	kvmPciBridges int
	timeout       time.Duration
	devicePath    string
}

var _ SpdkCsiInitiator = &xpuInitiator{}

// phyIDLock used to synchronize physical function usage
// between concurrent Connect{Nmve,VirtioBlk}() calls.
var phyIDLock sync.Mutex

func NewSpdkCsiXpuInitiator(volumeContext map[string]string, xpuConnClient *grpc.ClientConn, xpuTargetType string, kvmPciBridges int) (SpdkCsiInitiator, error) {
	targetInfo, err := parseSpdkXpuTargetType(xpuTargetType)
	if err != nil {
		return nil, err
	}
	xpu := &xpuInitiator{
		targetInfo:    targetInfo,
		volumeContext: volumeContext,
		kvmPciBridges: kvmPciBridges,
		timeout:       60 * time.Second,
	}

	storedXPUContext, _ := xpu.tryGetXPUContext() //nolint:errcheck // no need to check

	var backend XpuInitiator
	switch targetInfo.Backend {
	case xpuTargetBackendSma:
		backend, err = NewSpdkCsiSmaInitiator(volumeContext, xpuConnClient, targetInfo.TrType, storedXPUContext)
	case xpuTargetBackendOpi:
		backend, err = NewSpdkCsiOpiInitiator(volumeContext, xpuConnClient, targetInfo.TrType, storedXPUContext)
	default:
		return nil, fmt.Errorf("unknown target type: %q", targetInfo.Backend)
	}
	if err != nil {
		return nil, err
	}
	xpu.backend = backend
	return xpu, nil
}

func (xpu *xpuInitiator) saveXPUContext() error {
	xPUContext := xpu.backend.GetParam()
	xPUContext["devicePath"] = xpu.devicePath

	klog.Infof("saveXPUContext '%s' to the local file", xPUContext)
	err := StashXPUContext(
		xPUContext,
		xpu.volumeContext["stagingParentPath"],
	)
	if err != nil {
		return fmt.Errorf("saveXPUContext() error: %w", err)
	}
	return nil
}

func (xpu *xpuInitiator) tryGetXPUContext() (map[string]string, error) {
	xPUContext, err := LookupXPUContext(xpu.volumeContext["stagingParentPath"])
	if err != nil {
		return nil, fmt.Errorf("loadXPUContext() error: %w", err)
	}
	xpu.devicePath = xPUContext["devicePath"]
	return xPUContext, nil
}

func (xpu *xpuInitiator) Connect( /*ctx *context.Context*/ ) (string, error) {
	ctx, cancel := xpu.ctxTimeout()
	defer cancel()

	var devicePath string
	var err error
	switch xpu.targetInfo.TrType {
	case TransportTypeNvmfTCP:
		devicePath, err = xpu.ConnectNvmfTCP(ctx)
	case TransportTypeNvme:
		devicePath, err = xpu.ConnectNvme(ctx)
	case TransportTypeVirtioBlk:
		devicePath, err = xpu.ConnectVirtioBlk(ctx)
	default:
		return "", fmt.Errorf("unsupported xpu transport type %q", xpu.targetInfo.TrType)
	}

	if err != nil {
		return "", fmt.Errorf("XPU.Connect with transport type %s err %w", xpu.targetInfo.TrType, err)
	}

	err = xpu.saveXPUContext()
	if err != nil {
		return "", fmt.Errorf("XPU.Connect saveXPUContext err %w", err)
	}

	return devicePath, err
}

func (xpu *xpuInitiator) ConnectNvmfTCP(ctx context.Context) (string, error) {
	if err := xpu.backend.Connect(ctx, &ConnectParams{tcpTargetPort: smaNvmfTCPTargetPort}); err != nil {
		return "", err
	}

	// Initiate target connection with cmd:
	// nvme connect -t tcp -a "127.0.0.1" -s 4421 -n "nqn.2022-04.io.spdk.csi:cnode0:uuid:*"
	//nolint: contextcheck, gocritic
	devicePath, err := newInitiatorNVMf(xpu.volumeContext["model"]).Connect()
	if err != nil {
		// Call Disconnect(), to clean up if nvme connect failed, while CreateDevice and AttachVolume succeeded
		if errx := xpu.backend.Disconnect(ctx); errx != nil {
			klog.Errorf("clean up error: %s", errx)
		}
		return "", err
	}
	xpu.devicePath = devicePath

	return devicePath, nil
}

// ConnectNvme connects to volume using one of 'vfiouser' transport protocol.
// It uses for the available PCI bridge function for connecting the device.
// On success it returns the block device path on host.
func (xpu *xpuInitiator) ConnectNvme(ctx context.Context) (string, error) {
	devicePath, err := CheckIfNvmeDeviceExists(xpu.volumeContext["model"], nil)
	if devicePath != "" {
		klog.Infof("Found existing device for '%s': %v", xpu.volumeContext["mode"], devicePath)
		return devicePath, nil
	}
	if !os.IsNotExist(err) {
		klog.Errorf("failed to detect existing nvme device for '%s'", xpu.volumeContext["model"])
	}

	var pf uint32
	var vf uint32
	var vPf uint32

	phyIDLock.Lock()
	defer phyIDLock.Unlock()
	pf, vf, err = GetAvailablePhysicalFunction(xpu.kvmPciBridges)
	if err != nil {
		return "", fmt.Errorf("failed to detect free Nvme virtual function: %w", err)
	}

	// Both SMA and OPI always expect Vf value as 0 and detect the right KVM bus from Pf value.
	vPf = pf*32 + vf
	klog.Infof("Using next available functions, pf: %d, vf: %d", pf, vf)

	// Ask the backed to connect to the volume
	if err = xpu.backend.Connect(ctx, &ConnectParams{vPf: vPf}); err != nil {
		return "", err
	}

	bdf := fmt.Sprintf("0000:%02x:%02x.0", pf+1, vf)
	klog.Infof("Waiting till the device is ready for '%s' at '%s' ...", xpu.volumeContext["model"], bdf)
	devicePath, err = GetNvmeDeviceName(xpu.volumeContext["model"], bdf)
	if err != nil {
		klog.Errorf("Could not detect the device: %s", err)
		if errx := xpu.backend.Disconnect(ctx); errx != nil {
			klog.Errorf("failed to disconnect device: %v", err)
		}
		return "", err
	}
	klog.Infof("Device %s ready for '%s' at '%s'", devicePath, xpu.volumeContext["model"], bdf)
	xpu.devicePath = devicePath

	return devicePath, nil
}

// ConnectVirtioBlk connects to volume using one of 'virtio-blk' transport protocol.
// It uses for the available PCI bridge function for connecting the device.
// On success it returns the block device path on host.
func (xpu *xpuInitiator) ConnectVirtioBlk(ctx context.Context) (string, error) {
	var pf uint32
	var vf uint32
	var vPf uint32

	phyIDLock.Lock()
	defer phyIDLock.Unlock()

	var err error
	pf, vf, err = GetAvailablePhysicalFunction(xpu.kvmPciBridges)
	if err != nil {
		return "", fmt.Errorf("failed to detect free NVMe virtual function: %w", err)
	}
	// Both SMA and OPI always expect Vf value as 0 and detect the right KVM bus from Pf value.
	vPf = pf*32 + vf
	klog.Infof("Using next available functions, pf: %d, vf: %d", pf, vf)

	// Ask the backed to connect to the volume
	if err = xpu.backend.Connect(ctx, &ConnectParams{vPf: vPf}); err != nil {
		return "", err
	}

	bdf := fmt.Sprintf("0000:%02x:%02x.0", pf+1, vf)
	klog.Infof("Waiting still device ready for '%s' at '%s' ...", xpu.volumeContext["model"], bdf)
	var devicePath string
	devicePath, err = GetVirtioBlkDeviceName(bdf, true)
	if err != nil {
		klog.Errorf("Could not detect the device: %s", err)
		if errx := xpu.backend.Disconnect(ctx); errx != nil {
			klog.Errorf("failed to disconnect device: %v", errx)
		}
		return "", err
	}
	klog.Infof("Device %s ready for '%s' at '%s'", devicePath, xpu.volumeContext["model"], bdf)
	xpu.devicePath = devicePath

	return devicePath, nil
}

func (xpu *xpuInitiator) Disconnect( /*ctx context.Context*/ ) error {
	ctx, cancel := xpu.ctxTimeout()
	defer cancel()

	var err error
	err = fmt.Errorf("unsupported xpu transport type %q", xpu.targetInfo.TrType)
	switch xpu.targetInfo.TrType {
	case TransportTypeNvmfTCP:
		err = xpu.DisconnectNvmfTCP(ctx)
	case TransportTypeNvme:
		err = xpu.DisconnectNvme(ctx)
	case TransportTypeVirtioBlk:
		err = xpu.DisconnectVirtioBlk(ctx)
	}

	_ = CleanUpXPUContext(xpu.volumeContext["stagingParentPath"]) //nolint:errcheck // no need to check

	return err
}

// DisconnectNvmTCP disconnects volume. First it executes "nvme disconnect"
// to terminate the target connection and then ask the backend to drop the
// device.
func (xpu *xpuInitiator) DisconnectNvmfTCP(ctx context.Context) error {
	// nvme disconnect -n "nqn.2022-04.io.spdk.csi:cnode0:uuid:*"
	//nolint: contextcheck, gocritic
	if err := newInitiatorNVMf(xpu.volumeContext["model"]).Disconnect(); err != nil {
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	return xpu.backend.Disconnect(ctx)
}

// DisconnectNvme disconnects the target nvme device
func (xpu *xpuInitiator) DisconnectNvme(ctx context.Context) error {
	devicePath, err := CheckIfNvmeDeviceExists(xpu.volumeContext["model"], nil)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := xpu.backend.Disconnect(ctx); err != nil {
		return err
	}

	return waitForDeviceGone(devicePath)
}

// DisconnectVirtioBlk disconnects the target virtio-blk device
func (xpu *xpuInitiator) DisconnectVirtioBlk(ctx context.Context) error {
	klog.Infof("xpu Disconnect virtioblk device '%s'", xpu.devicePath)
	if xpu.devicePath == "" {
		return fmt.Errorf("failed to get block device path")
	}
	if err := xpu.backend.Disconnect(ctx); err != nil {
		return err
	}

	return waitForDeviceGone(xpu.devicePath)
}

func parseSpdkXpuTargetType(xpuTargetType string) (*XpuTargetType, error) {
	parts := strings.Split(xpuTargetType, "-")
	if parts[0] != "xpu" || len(parts) != 3 {
		return nil, fmt.Errorf("invalid xpuTargetType %q", xpuTargetType)
	}

	return &XpuTargetType{Backend: parts[1], TrType: TransportType(parts[2])}, nil
}

func (xpu *xpuInitiator) ctxTimeout() (context.Context, context.CancelFunc) {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), xpu.timeout)
	return ctxTimeout, cancel
}

// re-use the Connect() and Disconnect() functions from initiator.go
func newInitiatorNVMf(model string) *initiatorNVMf {
	return &initiatorNVMf{
		targetType: xpuNvmfTCPTargetType,
		targetAddr: xpuNvmfTCPTargetAddr,
		targetPort: xpuNvmfTCPTargetPort,
		nqn:        xpuNvmfTCPSubNqnPref + model,
		model:      model,
	}
}
