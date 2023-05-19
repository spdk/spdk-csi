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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"k8s.io/klog"
)

func ParseJSONFile(fileName string, result interface{}) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(bytes, result)
}

// round up bytes to megabytes
func ToMiB(bytes int64) int64 {
	const mi = 1024 * 1024
	return (bytes + mi - 1) / mi
}

// ${env:-def}
func FromEnv(env, def string) string {
	s := os.Getenv(env)
	if s != "" {
		return s
	}
	return def
}

// a trivial trylock implementation
type TryLock struct {
	locked int32
}

// acquire lock w/o waiting, return true if acquired, false otherwise
func (lock *TryLock) Lock() bool {
	// golang CAS forces sequential consistent memory order
	return atomic.CompareAndSwapInt32(&lock.locked, 0, 1)
}

// release lock
func (lock *TryLock) Unlock() {
	// golang atomic store forces release memory order
	atomic.StoreInt32(&lock.locked, 0)
}

var (
	nvmeReDeviceSysFileName = regexp.MustCompile(`nvme(\d+)n(\d+)|nvme(\d+)c(\d+)n(\d+)`)
	nvmeReDeviceName        = regexp.MustCompile(`c(\d+)`)
)

// getNvmeDeviceName checks the contents of given uuidFilePath for matching with
// nvmeModel. If it matches then returns the appropriate device name like nvme0n1
func getNvmeDeviceName(uuidFilePath, nvmeModel string) (string, error) {
	uuidContent, err := os.ReadFile(uuidFilePath)
	if err != nil {
		// a uuid file could be removed because of Disconnect() operation at the same time when doing ReadFile
		klog.Errorf("open uuid file uuidFilePath (%s) error: %s", uuidFilePath, err)
		return "", err
	}

	if strings.TrimSpace(string(uuidContent)) == nvmeModel {
		// Obtain the part nvme*c*n* or nvme*n* from the file path, eg, nvme0c0n1
		deviceSysFileName := nvmeReDeviceSysFileName.FindString(uuidFilePath)
		// Remove c* from (nvme*c*n*), eg, c0
		return nvmeReDeviceName.ReplaceAllString(deviceSysFileName, ""), nil
	}

	return "", fmt.Errorf("does not match")
}

func CheckIfNvmeDeviceExists(nvmeModel string, ignorePaths map[string]struct{}) (string, error) {
	uuidFilePaths, err := filepath.Glob("/sys/bus/pci/devices/*/nvme/nvme*/nvme*n*/uuid")
	if err != nil {
		return "", fmt.Errorf("obtain uuid files error: %w", err)
	}

	// The content of uuid file should be in the form of, eg, "b9e38b18-511e-429d-9660-f665fa7d63d0\n", which is also the volumeId.
	for _, filePath := range uuidFilePaths {
		if ignorePaths != nil {
			if _, visited := ignorePaths[filePath]; visited {
				continue
			}
			ignorePaths[filePath] = struct{}{}
		}
		deviceName, err := getNvmeDeviceName(filePath, nvmeModel)
		if err != nil {
			klog.Infof("Ignoring err: %v", err)
		}
		if deviceName != "" {
			return deviceName, nil
		}
	}
	return "", os.ErrNotExist
}

// detectNvemeDeviceName detects the device name in sysfs for given nvmeModel
func detectNvmeDeviceName(nvmeModel string) (string, error) {
	uuidFilePathsReadFlag := make(map[string]struct{})

	// Set 20 seconds timeout at maximum to try to find the exact device name for SMA Nvme
	for second := 0; second < 20; second++ {
		deviceName, err := CheckIfNvmeDeviceExists(nvmeModel, uuidFilePathsReadFlag)
		if err != nil {
			klog.Infof("detect nvme device '%s': %v", nvmeModel, err)
		} else {
			return deviceName, nil
		}
		// Wait a second before retry
		time.Sleep(time.Second)
	}

	return "", os.ErrDeadlineExceeded
}

// get the Nvme block device
func GetNvmeDeviceName(nvmeModel, bdf string) (string, error) {
	var deviceName string
	var err error
	if bdf != "" {
		var uuidFilePath string
		// find the uuid file path for the nvme device based on the bdf
		uuidFilePath, err = waitForDeviceReady(fmt.Sprintf("/sys/bus/pci/devices/%s/nvme/nvme*/nvme*n*/uuid", bdf))
		if err != nil {
			return "", fmt.Errorf("failed find device at %s: %w", uuidFilePath, err)
		}
		klog.Infof("uuidFilePath is %s", uuidFilePath)
		deviceName, err = getNvmeDeviceName(uuidFilePath, nvmeModel)
	} else {
		deviceName, err = detectNvmeDeviceName(nvmeModel)
	}
	if err != nil {
		return "", fmt.Errorf("failed to find nvme device name: %w", err)
	}

	return waitForDeviceReady("/dev/" + deviceName)
}

// GetNvmeAvailableFunction returns next available Pf and Vf by checking
// into sysfs for existing NVMe PCIe devices
func GetNvmeAvailableFunction(kvmBridgeCount int) (pf, vf uint32, err error) {
	for pf = 1; pf <= uint32(kvmBridgeCount); pf++ {
		for vf = 0; vf < 32; vf++ { // Assumption is that each PCI bridge supports
			devicePaths, err := filepath.Glob(fmt.Sprintf("/sys/bus/pci/devices/0000:%02x:%02x.*", pf, vf))
			if err != nil {
				return 0, 0, fmt.Errorf("sysfs failure: %w", err)
			}
			if devicePaths == nil {
				// No matching NVMe files found in sysfs, hence use
				// the first available pf/vf
				return pf - 1, vf, nil
			}
		}
	}

	return 0, 0, os.ErrNotExist
}
