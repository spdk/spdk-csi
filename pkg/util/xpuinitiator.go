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
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"k8s.io/klog"
)

const (
	xpuTargetBackendSma = "sma"
	xpuTargetBackendOpi = "opi"
)

const (
	xpuNvmfTCPTargetType = "tcp"
	xpuNvmfTCPAdrFam     = "ipv4"
	xpuNvmfTCPTargetAddr = "127.0.0.1"
	xpuNvmfTCPTargetPort = "4421"
	xpuNvmfTCPSubNqnPref = "nqn.2022-04.io.spdk.csi:cnode0:uuid:"
)

const (
	TransportTypeNvmfTCP   = "nvmftcp"
	TransportTypeNvme      = "nvme"
	TransportTypeVirtioBlk = "virtioblk"
)

type TransportType string

type XpuInitiator interface {
	Connect(context.Context, *ConnectParams) error
	Disconnect(context.Context) error
	GetParam() map[string]string
}

type ConnectParams struct {
	tcpTargetPort string // NvmfTCP
	PciEndpoint   PciEndpoint
}

type XpuTargetType struct {
	Backend string
	TrType  TransportType
}

type xpuInitiator struct {
	backend       XpuInitiator
	targetInfo    *XpuTargetType
	volumeContext map[string]string
	xpuConfig     XpuConfig
	timeout       time.Duration
	devicePath    string
	PciEndpoint   PciEndpoint
	PciBdfs       PciBdfs
}

type PciEndpoint struct {
	pfID uint32 // VirtioBlk, and Vfiouser
	vfID uint32 // VirtioBlk, and Vfiouser
}

type PciBdfs struct {
	pfBdf string // xPU hardware
	vfBdf string // xPU hardware and kvm
}

//nolint:tagliatelle // not using json:snake case
type PciIDs struct {
	VendorID string `json:"vendorID"`
	DeviceID string `json:"deviceID"`
	ClassID  string `json:"classID"`
}

var _ SpdkCsiInitiator = &xpuInitiator{}

// phyIDLock used to synchronize physical function usage
// between concurrent Connect{Nvme,VirtioBlk}() calls.
var phyIDLock sync.Mutex

//nolint:tagliatelle // not using json:snake case
type XpuConfig struct {
	Name       string `json:"name"`
	TargetType string `json:"targetType"`
	TargetAddr string `json:"targetAddr"`
	PciIDs     PciIDs `json:"pciIDs,omitempty"` // set omitempty in case of sma nvmf-tcp
}

func NewSpdkCsiXpuInitiator(volumeContext map[string]string, xpuConnClient *grpc.ClientConn, xpuConfig *XpuConfig) (SpdkCsiInitiator, error) {
	targetInfo, err := parseSpdkXpuTargetType(xpuConfig.TargetType)
	if err != nil {
		return nil, err
	}
	xpu := &xpuInitiator{
		targetInfo:    targetInfo,
		volumeContext: volumeContext,
		xpuConfig:     *xpuConfig,
		timeout:       60 * time.Second,
	}

	storedXpuContext, _ := xpu.tryGetXpuContext() //nolint:errcheck // no need to check

	var backend XpuInitiator
	switch targetInfo.Backend {
	case xpuTargetBackendSma:
		backend, err = NewSpdkCsiSmaInitiator(volumeContext, xpuConnClient, targetInfo.TrType, storedXpuContext)
	case xpuTargetBackendOpi:
		backend, err = NewSpdkCsiOpiInitiator(volumeContext, xpuConnClient, targetInfo.TrType, storedXpuContext)
	default:
		return nil, fmt.Errorf("unknown target type: %q", targetInfo.Backend)
	}
	if err != nil {
		return nil, err
	}
	xpu.backend = backend
	return xpu, nil
}

func (xpu *xpuInitiator) saveXpuContext() error {
	xPUContext := xpu.backend.GetParam()
	xPUContext["devicePath"] = xpu.devicePath
	xPUContext["pfID"] = fmt.Sprintf("%d", xpu.PciEndpoint.pfID)
	xPUContext["vfID"] = fmt.Sprintf("%d", xpu.PciEndpoint.vfID)
	xPUContext["pfBdf"] = xpu.PciBdfs.pfBdf
	xPUContext["vfBdf"] = xpu.PciBdfs.vfBdf

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

func (xpu *xpuInitiator) tryGetXpuContext() (map[string]string, error) {
	xPUContext, err := LookupXPUContext(xpu.volumeContext["stagingParentPath"])
	if err != nil {
		return nil, fmt.Errorf("loadXPUContext() error: %w", err)
	}
	xpu.devicePath = xPUContext["devicePath"]
	xpu.PciBdfs.pfBdf = xPUContext["pfBdf"]
	xpu.PciBdfs.vfBdf = xPUContext["vfBdf"]
	pfID64, _ := strconv.ParseUint(xPUContext["pfID"], 10, 64) //nolint:errcheck // no need to check
	xpu.PciEndpoint.pfID = uint32(pfID64)
	vfID64, _ := strconv.ParseUint(xPUContext["vfID"], 10, 64) //nolint:errcheck // no need to check
	xpu.PciEndpoint.vfID = uint32(vfID64)

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

	err = xpu.saveXpuContext()
	if err != nil {
		return "", fmt.Errorf("XPU.Connect saveXPUContext err %w", err)
	}

	return devicePath, err
}

func (xpu *xpuInitiator) ConnectNvmfTCP(ctx context.Context) (string, error) {
	if err := xpu.backend.Connect(ctx, &ConnectParams{
		tcpTargetPort: smaNvmfTCPTargetPort,
	}); err != nil {
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

// In xpu hardware case with OPI, once Connect() succeeds, which means the VF is created successfully,
// the "nvme" will be required to be written to the file (/sys/bus/pci/devices/"$pfBdf"/virtfn"$vfID-1"/driver_override),
// and $vfBdf will be required to be written to the file (/sys/bus/pci/drivers/nvme/bind).
// More information could be seen: https://github.com/opiproject/opi-intel-bridge/blob/0f2c034da270367a6b3078d170afe21a56b86b04/README.md?plain=1#L166
func (xpu *xpuInitiator) updateNvmeFilesForConnect(pe PciEndpoint, bdfs PciBdfs) error {
	if !IsKvm(&xpu.xpuConfig.PciIDs) && xpu.targetInfo.Backend == xpuTargetBackendOpi {
		if err := appendContentToFile(fmt.Sprintf("/sys/bus/pci/devices/%s/virtfn%d/driver_override", bdfs.pfBdf, pe.vfID-1), "nvme"); err != nil {
			klog.Errorf("write (nvme) to file (/sys/bus/pci/devices/%s/virtfn%d/driver_override) err: %v", bdfs.pfBdf, pe.vfID-1, err)
			return err
		}
		klog.Infof("write (nvme) to file (/sys/bus/pci/devices/%s/virtfn%d/driver_override) successfully", bdfs.pfBdf, pe.vfID-1)

		if err := appendContentToFile("/sys/bus/pci/drivers/nvme/bind", bdfs.vfBdf); err != nil {
			klog.Errorf("write vfBdf (%s) to file (/sys/bus/pci/drivers/nvme/bind) err: %v", bdfs.vfBdf, err)
			return err
		}
		klog.Infof("write vfBdf (%s) to file (/sys/bus/pci/drivers/nvme/bind) successfully", bdfs.vfBdf)
	}
	return nil
}

// In xpu hardware case with OPI, before Disconnect() is called,
// $vfBdf is required to be written to the file (/sys/bus/pci/drivers/nvme/unbind),
// and "(null)" is required to be written to the file (/sys/bus/pci/devices/"$pfBdf"/virtfn"$vfID-1"/driver_override),
// More information could be seen: https://github.com/opiproject/opi-intel-bridge/blob/0f2c034da270367a6b3078d170afe21a56b86b04/README.md?plain=1#L174
func (xpu *xpuInitiator) updateNvmeFilesForDisconnect(pe PciEndpoint, bdfs PciBdfs) error {
	if !(xpu.xpuConfig.PciIDs == KvmPciBridgeIDs) && xpu.targetInfo.Backend == xpuTargetBackendOpi {
		if err := appendContentToFile("/sys/bus/pci/drivers/nvme/unbind", bdfs.vfBdf); err != nil {
			klog.Errorf("write vfBdf (%s) to file (/sys/bus/pci/drivers/nvme/unbind) err: %v", bdfs.vfBdf, err)
			return err
		}
		klog.Infof("write vfBdf (%s) to file (/sys/bus/pci/drivers/nvme/unbind) successfully", bdfs.vfBdf)

		if err := appendContentToFile(fmt.Sprintf("/sys/bus/pci/devices/%s/virtfn%d/driver_override", bdfs.pfBdf, pe.vfID-1), "(null)"); err != nil {
			klog.Errorf("write (null) to file (/sys/bus/pci/devices/%s/virtfn%d/driver_override) err: %v", bdfs.pfBdf, pe.vfID-1, err)
			return err
		}
		klog.Infof("write (null) to file (/sys/bus/pci/devices/%s/virtfn%d/driver_override) successfully", bdfs.pfBdf, pe.vfID-1)
	}
	return nil
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

	var pe PciEndpoint
	var bdfs PciBdfs

	phyIDLock.Lock()
	defer phyIDLock.Unlock()
	pe, bdfs, err = GetAvailableFunctions(&xpu.xpuConfig)
	if err != nil {
		return "", fmt.Errorf("failed to detect free NVMe virtual function: %w", err)
	}
	klog.Infof("Using next available physical function: %d, virtual function: %d", pe.pfID, pe.vfID)
	xpu.PciBdfs = bdfs
	xpu.PciEndpoint = pe

	// Ask the backed to connect to the volume
	if err = xpu.backend.Connect(ctx, &ConnectParams{
		PciEndpoint: pe,
	}); err != nil {
		return "", err
	}

	// Update Nvme file for OPI case with xpu hardware
	if err = xpu.updateNvmeFilesForConnect(pe, bdfs); err != nil {
		return "", err
	}

	klog.Infof("Waiting till the device is ready for '%s' at '%s' ...", xpu.volumeContext["model"], bdfs.vfBdf)

	devicePath, err = GetNvmeDeviceName(xpu.volumeContext["model"], bdfs.vfBdf)
	if err != nil {
		klog.Errorf("Could not detect the device: %s", err)
		if errx := xpu.backend.Disconnect(ctx); errx != nil {
			klog.Errorf("failed to disconnect device: %v", errx)
		}
		return "", err
	}
	klog.Infof("Device %s ready for '%s' at '%s'", devicePath, xpu.volumeContext["model"], bdfs.vfBdf)
	xpu.devicePath = devicePath

	return devicePath, nil
}

// ConnectVirtioBlk connects to volume using one of 'virtio-blk' transport protocol.
// It uses for the available PCI bridge function for connecting the device.
// On success it returns the block device path on host.
func (xpu *xpuInitiator) ConnectVirtioBlk(ctx context.Context) (string, error) {
	var pe PciEndpoint
	var bdfs PciBdfs

	phyIDLock.Lock()
	defer phyIDLock.Unlock()

	var err error
	pe, bdfs, err = GetAvailableFunctions(&xpu.xpuConfig)
	if err != nil {
		return "", fmt.Errorf("failed to detect free NVMe virtual function: %w", err)
	}
	klog.Infof("Using next available physical function: %d, virtual function: %d", pe.pfID, pe.vfID)
	xpu.PciBdfs = bdfs
	xpu.PciEndpoint = pe

	// Ask the backed to connect to the volume
	if err = xpu.backend.Connect(ctx, &ConnectParams{
		PciEndpoint: pe,
	}); err != nil {
		return "", err
	}

	var devicePath string
	devicePath, err = GetVirtioBlkDeviceName(bdfs.vfBdf, true)
	if err != nil {
		klog.Errorf("Could not detect the device: %s", err)
		if errx := xpu.backend.Disconnect(ctx); errx != nil {
			klog.Errorf("failed to disconnect device: %v", errx)
		}
		return "", err
	}
	klog.Infof("Device %s ready for '%s' at '%s'", devicePath, xpu.volumeContext["model"], bdfs.vfBdf)
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
	klog.Infof("xpu Disconnect nvme device '%s'", xpu.devicePath)
	if xpu.devicePath == "" {
		return fmt.Errorf("failed to get block device path")
	}

	// Update Nvme file for OPI case with xpu hardware
	if err := xpu.updateNvmeFilesForDisconnect(xpu.PciEndpoint, xpu.PciBdfs); err != nil {
		return err
	}

	if err := xpu.backend.Disconnect(ctx); err != nil {
		return err
	}

	return waitForDeviceGone(xpu.devicePath)
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
