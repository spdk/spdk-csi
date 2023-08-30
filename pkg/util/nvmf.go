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
	"strconv"
	"strings"
	"sync/atomic"

	"k8s.io/klog"
)

type nodeNVMf struct {
	client *rpcClient

	targetType   string // RDMA, TCP
	targetAddr   string
	targetPort   string
	transCreated int32
}

func newNVMf(client *rpcClient, targetType, targetAddr string) *nodeNVMf {
	return &nodeNVMf{
		client:     client,
		targetType: targetType,
		targetAddr: targetAddr,
		targetPort: cfgNVMfSvcPort,
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
	lvol, err := node.client.getVolume(lvolID)
	if err != nil {
		return nil, err
	}
	lvStore, err := node.client.getLvstore(lvolID)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"targetType": node.targetType,
		"targetAddr": node.targetAddr,
		"targetPort": node.targetPort,
		"nqn":        node.getVolumeNqn(lvolID),
		"model":      node.getVolumeModel(lvolID),
		"lvolSize":   strconv.FormatInt(lvol.BlockSize*lvol.NumBlocks, 10),
		"lvstore":    lvStore,
	}, nil
}

// CreateVolume creates a logical volume and returns volume ID
func (node *nodeNVMf) CreateVolume(lvolName, lvsName string, sizeMiB int64) (string, error) {
	// all volume have an alias ID named lvsName/lvolName
	lvol, err := node.client.getVolume(fmt.Sprintf("%s/%s", lvsName, lvolName))
	if err == nil {
		klog.Warningf("volume already created: %s/%s %s", lvsName, lvolName, lvol.UUID)
		return lvol.UUID, nil
	}

	lvolID, err := node.client.createVolume(lvolName, lvsName, sizeMiB)
	if err != nil {
		return "", err
	}
	klog.V(5).Infof("volume created: %s", lvolID)
	return lvolID, nil
}

// CloneVolume creates a logical volume based on the source volume and returns volume ID
func (node *nodeNVMf) CloneVolume(lvolName, lvsName, sourceLvolID string) (string, error) {
	// all volume have an alias ID named lvsName/lvolName
	lvol, err := node.client.getVolume(fmt.Sprintf("%s/%s", lvsName, lvolName))
	if err == nil {
		klog.Warningf("volume already cloned: %s/%s %s", lvsName, lvolName, lvol.UUID)
		return lvol.UUID, nil
	}
	var lvolID string
	// clone sourceLvol
	lvolID, err = node.client.cloneVolume(lvolName, sourceLvolID)

	if err != nil {
		return "", err
	}
	klog.V(5).Infof("volume cloned: %s", lvolID)
	return lvolID, nil
}

// GetVolume returns the volume id of the given volume name and lvstore name. return error if not found.
func (node *nodeNVMf) GetVolume(lvolName, lvsName string) (string, error) {
	lvol, err := node.client.getVolume(fmt.Sprintf("%s/%s", lvsName, lvolName))
	if err != nil {
		return "", err
	}
	return lvol.UUID, err
}

func (node *nodeNVMf) isVolumeCreated(lvolID string) (bool, error) {
	return node.client.isVolumeCreated(lvolID)
}

func (node *nodeNVMf) CreateSnapshot(lvolName, snapshotName string) (string, error) {
	lvsName, err := node.client.getLvstore(lvolName)
	if err != nil {
		return "", err
	}
	lvol, err := node.client.getVolume(fmt.Sprintf("%s/%s", lvsName, snapshotName))
	if err == nil {
		klog.Warningf("snapshot already created: %s", lvol.UUID)
		return lvol.UUID, nil
	}
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
	klog.V(5).Infof("volume deleted: %s", lvolID)
	return nil
}

// PublishVolume exports a volume through NVMf target
func (node *nodeNVMf) PublishVolume(lvolID string) error {
	exists, err := node.isVolumeCreated(lvolID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrVolumeDeleted
	}
	published, err := node.isVolumePublished(lvolID)
	if err != nil {
		return err
	}
	if published {
		return nil
	}

	err = node.createTransport()
	if err != nil {
		return err
	}

	err = node.createSubsystem(lvolID)
	if err != nil {
		return err
	}

	_, err = node.subsystemAddNs(lvolID)
	if err != nil {
		node.deleteSubsystem(lvolID) //nolint:errcheck // we can do few
		return err
	}

	err = node.subsystemAddListener(lvolID)
	if err != nil {
		node.subsystemRemoveNs(lvolID) //nolint:errcheck // ditto
		node.deleteSubsystem(lvolID)   //nolint:errcheck // ditto
		return err
	}

	klog.V(5).Infof("volume published: %s", lvolID)
	return nil
}

func (node *nodeNVMf) isVolumePublished(lvolID string) (bool, error) {
	var result []struct {
		Address struct {
			TrType  string `json:"trtype"`
			AdrFam  string `json:"adrfam"`
			TrAddr  string `json:"traddr"`
			TrSvcID string `json:"trsvcid"`
		} `json:"address"`
	}
	params := struct {
		Nqn string `json:"nqn"`
	}{
		Nqn: node.getVolumeNqn(lvolID),
	}
	err := node.client.call("nvmf_subsystem_get_listeners", &params, &result)
	if err != nil {
		// querying nqn that does not exist, an invalid parameters error will be thrown
		if errorMatches(err, ErrInvalidParameters) {
			return false, nil
		}
		return false, err
	}
	for i := range result {
		if result[i].Address.TrType == node.targetType &&
			result[i].Address.TrAddr == node.targetAddr &&
			result[i].Address.TrSvcID == node.targetPort &&
			result[i].Address.AdrFam == cfgAddrFamily {
			return true, nil
		}
	}
	return false, nil
}

func (node *nodeNVMf) UnpublishVolume(lvolID string) error {
	exists, err := node.isVolumeCreated(lvolID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrVolumeDeleted
	}
	published, err := node.isVolumePublished(lvolID)
	if err != nil {
		return err
	}
	if !published {
		// already unpublished
		return nil
	}
	err = node.subsystemRemoveNs(lvolID)
	if err != nil {
		// we should try deleting subsystem even if we fail here
		klog.Errorf("failed to remove namespace(nqn=%s): %s", node.getVolumeNqn(lvolID), err)
	}
	err = node.deleteSubsystem(lvolID)
	if err != nil {
		return err
	}
	klog.V(5).Infof("volume unpublished: %s", lvolID)
	return nil
}

func (node *nodeNVMf) getVolumeModel(lvolID string) string {
	return lvolID
}

func (node *nodeNVMf) getVolumeNqn(lvolID string) string {
	return "nqn.2020-04.io.spdk.csi:uuid:" + node.getVolumeModel(lvolID)
}

func (node *nodeNVMf) createSubsystem(lvolID string) error {
	params := struct {
		Nqn          string `json:"nqn"`
		AllowAnyHost bool   `json:"allow_any_host"`
		SerialNumber string `json:"serial_number"`
		ModelNumber  string `json:"model_number"`
	}{
		Nqn:          node.getVolumeNqn(lvolID),
		AllowAnyHost: cfgAllowAnyHost,
		SerialNumber: "spdkcsi-sn",
		ModelNumber:  node.getVolumeModel(lvolID), // client matches imported disk with model string
	}

	return node.client.call("nvmf_create_subsystem", &params, nil)
}

func (node *nodeNVMf) subsystemAddNs(lvolID string) (int, error) {
	type namespace struct {
		BdevName string `json:"bdev_name"`
	}

	params := struct {
		Nqn       string    `json:"nqn"`
		Namespace namespace `json:"namespace"`
	}{
		Nqn: node.getVolumeNqn(lvolID),
		Namespace: namespace{
			BdevName: lvolID,
		},
	}
	var nsID int
	err := node.client.call("nvmf_subsystem_add_ns", &params, &nsID)
	return nsID, err
}

func (node *nodeNVMf) subsystemGetNsID(lvolID string) (int, error) {
	var results []struct {
		Nqn       string `json:"nqn"`
		Namespace []struct {
			NSID     int    `json:"nsid"`
			BdevName string `json:"bdev_name"`
		} `json:"namespaces"`
	}
	err := node.client.call("nvmf_get_subsystems", nil, &results)
	if err != nil {
		return 0, err
	}
	nqn := node.getVolumeNqn(lvolID)
	for i := range results {
		result := &results[i]
		if result.Nqn == nqn {
			for i := range result.Namespace {
				if result.Namespace[i].BdevName == lvolID {
					return result.Namespace[i].NSID, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("no such namespace")
}

func (node *nodeNVMf) subsystemAddListener(lvolID string) error {
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
		Nqn: node.getVolumeNqn(lvolID),
		ListenAddress: listenAddress{
			TrType:  node.targetType,
			TrAddr:  node.targetAddr,
			TrSvcID: node.targetPort,
			AdrFam:  cfgAddrFamily,
		},
	}

	return node.client.call("nvmf_subsystem_add_listener", &params, nil)
}

func (node *nodeNVMf) subsystemRemoveNs(lvolID string) error {
	nsID, err := node.subsystemGetNsID(lvolID)
	if err != nil {
		return err
	}

	params := struct {
		Nqn  string `json:"nqn"`
		NsID int    `json:"nsid"`
	}{
		Nqn:  node.getVolumeNqn(lvolID),
		NsID: nsID,
	}
	return node.client.call("nvmf_subsystem_remove_ns", &params, nil)
}

func (node *nodeNVMf) deleteSubsystem(lvolID string) error {
	params := struct {
		Nqn string `json:"nqn"`
	}{
		Nqn: node.getVolumeNqn(lvolID),
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
