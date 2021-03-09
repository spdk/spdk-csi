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

package util

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/klog"
)

// SpdkCsiInitiator defines interface for NVMeoF/iSCSI initiator
//
// - Connect initiates target connection and returns local block device filename
//   e.g., /dev/disk/by-id/nvme-SPDK_Controller1_SPDK00000000000001
// - Disconnect terminates target connection
//
// - Caller(node service) should serialize calls to same initiator
// - Implementation should be idempotent to duplicated requests
type SpdkCsiInitiator interface {
	Connect() (string, error)
	Disconnect() error
}

func NewSpdkCsiInitiator(volumeContext map[string]string) (SpdkCsiInitiator, error) {
	targetType := strings.ToLower(volumeContext["targetType"])
	switch targetType {
	case "rdma", "tcp":
		return &initiatorNVMf{
			// see util/nvmf.go VolumeInfo()
			targetType: volumeContext["targetType"],
			targetAddr: volumeContext["targetAddr"],
			targetPort: volumeContext["targetPort"],
			nqn:        volumeContext["nqn"],
			model:      volumeContext["model"],
		}, nil
	case "iscsi":
		return &initiatorISCSI{
			targetAddr: volumeContext["targetAddr"],
			targetPort: volumeContext["targetPort"],
			iqn:        volumeContext["iqn"],
		}, nil
	default:
		return nil, fmt.Errorf("unknown initiator: %s", targetType)
	}
}

// NVMf initiator implementation
type initiatorNVMf struct {
	targetType string
	targetAddr string
	targetPort string
	nqn        string
	model      string
}

func (nvmf *initiatorNVMf) Connect() (string, error) {
	// nvme connect -t tcp -a 192.168.1.100 -s 4420 -n "nqn"
	cmdLine := []string{"nvme", "connect", "-t", strings.ToLower(nvmf.targetType),
		"-a", nvmf.targetAddr, "-s", nvmf.targetPort, "-n", nvmf.nqn}
	err := execWithTimeout(cmdLine, 40)
	if err != nil {
		// go on checking device status in case caused by duplicated request
		klog.Errorf("command %v failed: %s", cmdLine, err)
	}

	deviceGlob := fmt.Sprintf("/dev/disk/by-id/*%s*", nvmf.model)
	devicePath, err := waitForDeviceReady(deviceGlob, 20)
	if err != nil {
		return "", err
	}
	return devicePath, nil
}

func (nvmf *initiatorNVMf) Disconnect() error {
	// nvme disconnect -n "nqn"
	cmdLine := []string{"nvme", "disconnect", "-n", nvmf.nqn}
	err := execWithTimeout(cmdLine, 40)
	if err != nil {
		// go on checking device status in case caused by duplicate request
		klog.Errorf("command %v failed: %s", cmdLine, err)
	}

	deviceGlob := fmt.Sprintf("/dev/disk/by-id/*%s*", nvmf.model)
	return waitForDeviceGone(deviceGlob, 20)
}

type initiatorISCSI struct {
	targetAddr string
	targetPort string
	iqn        string
}

func (iscsi *initiatorISCSI) Connect() (string, error) {
	// iscsiadm -m discovery -t sendtargets -p ip:port
	target := iscsi.targetAddr + ":" + iscsi.targetPort
	cmdLine := []string{"iscsiadm", "-m", "discovery", "-t", "sendtargets", "-p", target}
	err := execWithTimeout(cmdLine, 40)
	if err != nil {
		klog.Errorf("command %v failed: %s", cmdLine, err)
	}
	// iscsiadm -m node -T "iqn" -p ip:port --login
	cmdLine = []string{"iscsiadm", "-m", "node", "-T", iscsi.iqn, "-p", target, "--login"}
	err = execWithTimeout(cmdLine, 40)
	if err != nil {
		klog.Errorf("command %v failed: %s", cmdLine, err)
	}

	deviceGlob := fmt.Sprintf("/dev/disk/by-path/*%s*", iscsi.iqn)
	devicePath, err := waitForDeviceReady(deviceGlob, 20)
	if err != nil {
		return "", err
	}
	return devicePath, nil
}

func (iscsi *initiatorISCSI) Disconnect() error {
	target := iscsi.targetAddr + ":" + iscsi.targetPort
	// iscsiadm -m node -T "iqn" -p ip:port --logout
	cmdLine := []string{"iscsiadm", "-m", "node", "-T", iscsi.iqn, "-p", target, "--logout"}
	err := execWithTimeout(cmdLine, 40)
	if err != nil {
		klog.Errorf("command %v failed: %s", cmdLine, err)
	}

	deviceGlob := fmt.Sprintf("/dev/disk/by-path/*%s*", iscsi.iqn)
	return waitForDeviceGone(deviceGlob, 20)
}

// wait for device file comes up or timeout
func waitForDeviceReady(deviceGlob string, seconds int) (string, error) {
	for i := 0; i <= seconds; i++ {
		time.Sleep(time.Second)
		matches, err := filepath.Glob(deviceGlob)
		if err != nil {
			return "", err
		}
		// two symbol links under /dev/disk/by-id/ to same device
		if len(matches) >= 1 {
			return matches[0], nil
		}
	}
	return "", fmt.Errorf("timed out waiting device ready: %s", deviceGlob)
}

// wait for device file gone or timeout
func waitForDeviceGone(deviceGlob string, seconds int) error {
	for i := 0; i <= seconds; i++ {
		time.Sleep(time.Second)
		matches, err := filepath.Glob(deviceGlob)
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return nil
		}
	}
	return fmt.Errorf("timed out waiting device gone: %s", deviceGlob)
}

// exec shell command with timeout(in seconds)
func execWithTimeout(cmdLine []string, timeout int) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	klog.Infof("running command: %v", cmdLine)
	cmd := exec.CommandContext(ctx, cmdLine[0], cmdLine[1:]...)
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out")
	}
	if output != nil {
		klog.Infof("command returned: %s", output)
	}
	return err
}
