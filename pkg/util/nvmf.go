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
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"k8s.io/klog"
)

const invalidNSID = 0

type nodeNVMf struct {
	client *rpcClient

	targetType   string // RDMA, TCP
	targetAddr   string
	targetPort   string
	transCreated int32

	lvols map[string]*lvolNVMf
	mtx   sync.Mutex // for concurrent access to lvols map
}

type lvolNVMf struct {
	nsID  int
	nqn   string
	model string
}

func (lvol *lvolNVMf) reset() {
	lvol.nsID = invalidNSID
	lvol.nqn = ""
	lvol.model = ""
}

func newNVMf(client *rpcClient, targetType, targetAddr string) *nodeNVMf {
	return &nodeNVMf{
		client:     client,
		targetType: targetType,
		targetAddr: targetAddr,
		targetPort: cfgNVMfSvcPort,
		lvols:      make(map[string]*lvolNVMf),
	}
}

func (node *nodeNVMf) Info() string {
	return node.client.info()
}

func (node *nodeNVMf) LvStores() ([]LvStore, error) {
	return node.client.lvStores()
}

// VolumeInfo returns a string:string map containing information necessary
// for CSI node(initiator) to connect to this target and identify the disk.
func (node *nodeNVMf) VolumeInfo(lvolID string) (map[string]string, error) {
	node.mtx.Lock()
	lvol, exists := node.lvols[lvolID]
	node.mtx.Unlock()

	if !exists {
		return nil, fmt.Errorf("volume not exists: %s", lvolID)
	}

	return map[string]string{
		"targetType": node.targetType,
		"targetAddr": node.targetAddr,
		"targetPort": node.targetPort,
		"nqn":        lvol.nqn,
		"model":      lvol.model,
	}, nil
}

// CreateVolume creates a logical volume and returns volume ID
func (node *nodeNVMf) CreateVolume(lvsName string, sizeMiB int64) (string, error) {
	lvolID, err := node.client.createVolume(lvsName, sizeMiB)
	if err != nil {
		return "", err
	}

	node.mtx.Lock()
	defer node.mtx.Unlock()

	_, exists := node.lvols[lvolID]
	if exists {
		return "", fmt.Errorf("volume ID already exists: %s", lvolID)
	}
	node.lvols[lvolID] = &lvolNVMf{nsID: invalidNSID}

	klog.V(5).Infof("volume created: %s", lvolID)
	return lvolID, nil
}

func (node *nodeNVMf) CreateSnapshot(lvolName, snapshotName string) (string, error) {
	snapshotID, err := node.client.snapshot(lvolName, snapshotName)
	if err != nil {
		return "", err
	}

	klog.V(5).Infof("snapshot created: %s", snapshotID)
	return snapshotID, nil
}

func (node *nodeNVMf) DeleteVolume(lvolID string) error {
	err := node.client.deleteVolume(lvolID)
	if err != nil {
		return err
	}

	node.mtx.Lock()
	defer node.mtx.Unlock()

	delete(node.lvols, lvolID)

	klog.V(5).Infof("volume deleted: %s", lvolID)
	return nil
}

// PublishVolume exports a volume through NVMf target
func (node *nodeNVMf) PublishVolume(lvolID string) error {
	var err error

	err = node.createTransport()
	if err != nil {
		return err
	}

	node.mtx.Lock()
	lvol, exists := node.lvols[lvolID]
	node.mtx.Unlock()

	if !exists {
		return ErrVolumeDeleted
	}
	if lvol.nqn != "" {
		return ErrVolumePublished
	}

	// cleanup lvol on error
	defer func() {
		if err != nil {
			lvol.reset()
		}
	}()

	lvol.model = lvolID
	lvol.nqn, err = node.createSubsystem(lvol.model)
	if err != nil {
		return err
	}

	lvol.nsID, err = node.subsystemAddNs(lvol.nqn, lvolID)
	if err != nil {
		node.deleteSubsystem(lvol.nqn) //nolint:errcheck // we can do few
		return err
	}

	err = node.subsystemAddListener(lvol.nqn)
	if err != nil {
		node.subsystemRemoveNs(lvol.nqn, lvol.nsID) //nolint:errcheck // ditto
		node.deleteSubsystem(lvol.nqn)              //nolint:errcheck // ditto
		return err
	}

	klog.V(5).Infof("volume published: %s", lvolID)
	return nil
}

func (node *nodeNVMf) UnpublishVolume(lvolID string) error {
	var err error

	node.mtx.Lock()
	lvol, exists := node.lvols[lvolID]
	node.mtx.Unlock()

	if !exists {
		return ErrVolumeDeleted
	}
	if lvol.nqn == "" {
		return ErrVolumeUnpublished
	}

	err = node.subsystemRemoveNs(lvol.nqn, lvol.nsID)
	if err != nil {
		// we should try deleting subsystem even if we fail here
		klog.Errorf("failed to remove namespace(nqn=%s, nsid=%d): %s", lvol.nqn, lvol.nsID, err)
	} else {
		lvol.nsID = invalidNSID
	}

	err = node.deleteSubsystem(lvol.nqn)
	if err != nil {
		return err
	}

	lvol.reset()
	klog.V(5).Infof("volume unpublished: %s", lvolID)
	return nil
}

func (node *nodeNVMf) createSubsystem(model string) (string, error) {
	nqn := "nqn.2020-04.io.spdk.csi:uuid:" + model

	params := struct {
		Nqn          string `json:"nqn"`
		AllowAnyHost bool   `json:"allow_any_host"`
		SerialNumber string `json:"serial_number"`
		ModelNumber  string `json:"model_number"`
	}{
		Nqn:          nqn,
		AllowAnyHost: cfgAllowAnyHost,
		SerialNumber: "spdkcsi-sn",
		ModelNumber:  model, // client matches imported disk with model string
	}

	err := node.client.call("nvmf_create_subsystem", &params, nil)
	if err != nil {
		return "", err
	}

	return nqn, nil
}

func (node *nodeNVMf) subsystemAddNs(nqn, lvolID string) (int, error) {
	type namespace struct {
		BdevName string `json:"bdev_name"`
	}

	params := struct {
		Nqn       string    `json:"nqn"`
		Namespace namespace `json:"namespace"`
	}{
		Nqn: nqn,
		Namespace: namespace{
			BdevName: lvolID,
		},
	}

	var nsID int

	err := node.client.call("nvmf_subsystem_add_ns", &params, &nsID)
	return nsID, err
}

func (node *nodeNVMf) subsystemAddListener(nqn string) error {
	type listenAddress struct {
		TrType  string `json:"trtype"`
		AdrFam  string `json:"adrfam"`
		TrAddr  string `json:"traddr"`
		TrSvcID string `json:"trsvcid"`
	}

	params := struct {
		Nqn           string        `json:"nqn"`
		ListenAddress listenAddress `json:"listen_address"`
	}{
		Nqn: nqn,
		ListenAddress: listenAddress{
			TrType:  node.targetType,
			TrAddr:  node.targetAddr,
			TrSvcID: node.targetPort,
			AdrFam:  cfgAddrFamily,
		},
	}

	return node.client.call("nvmf_subsystem_add_listener", &params, nil)
}

func (node *nodeNVMf) subsystemRemoveNs(nqn string, nsID int) error {
	params := struct {
		Nqn  string `json:"nqn"`
		NsID int    `json:"nsid"`
	}{
		Nqn:  nqn,
		NsID: nsID,
	}

	return node.client.call("nvmf_subsystem_remove_ns", &params, nil)
}

func (node *nodeNVMf) deleteSubsystem(nqn string) error {
	params := struct {
		Nqn string `json:"nqn"`
	}{
		Nqn: nqn,
	}

	return node.client.call("nvmf_delete_subsystem", &params, nil)
}

func (node *nodeNVMf) createTransport() error {
	// concurrent requests can happen despite this fast path check
	if atomic.LoadInt32(&node.transCreated) != 0 {
		return nil
	}

	// TODO: support transport parameters
	params := struct {
		TrType string `json:"trtype"`
	}{
		TrType: node.targetType,
	}

	err := node.client.call("nvmf_create_transport", &params, nil)

	if err == nil {
		klog.V(5).Infof("Transport created: %s,%s", node.targetAddr, node.targetType)
		atomic.StoreInt32(&node.transCreated, 1)
	} else if strings.Contains(err.Error(), "already exists") {
		err = nil // ignore transport already exists error
		atomic.StoreInt32(&node.transCreated, 1)
	}

	return err
}
