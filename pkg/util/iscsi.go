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
	"sync"

	"k8s.io/klog"
)

const (
	numberPortalGroupTag    = 1
	numberInitiatorGroupTag = 1
	targetQueueDepth        = 64
	// SPDK ISCSI Iqn fixed prefix
	iqnPrefixName = "iqn.2016-06.io.spdk:"
)

type nodeISCSI struct {
	client     *rpcClient
	targetAddr string
	targetPort string
	lvols      map[string]*lvolISCSI
	mtx        sync.Mutex // for concurrent access to lvols map
}

type lvolISCSI struct {
	published bool
}

func (lvol *lvolISCSI) reset() {
	lvol.published = false
}

func newISCSI(client *rpcClient, targetAddr string) *nodeISCSI {
	return &nodeISCSI{
		client:     client,
		targetAddr: targetAddr,
		targetPort: cfgISCSISvcPort,
		lvols:      make(map[string]*lvolISCSI),
	}
}

func (node *nodeISCSI) Info() string {
	return node.client.info()
}

func (node *nodeISCSI) LvStores() ([]LvStore, error) {
	return node.client.lvStores()
}

// VolumeInfo returns a string:string map containing information necessary
// for CSI node(initiator) to connect to this target and identify the disk.
func (node *nodeISCSI) VolumeInfo(lvolID string) (map[string]string, error) {
	node.mtx.Lock()
	_, exists := node.lvols[lvolID]
	node.mtx.Unlock()

	if !exists {
		return nil, fmt.Errorf("volume not exists: %s", lvolID)
	}

	return map[string]string{
		"targetAddr": node.targetAddr,
		"targetPort": node.targetPort,
		"iqn":        iqnPrefixName + lvolID,
		"targetType": "iscsi",
	}, nil
}

// CreateVolume creates a logical volume and returns volume ID
func (node *nodeISCSI) CreateVolume(lvsName string, sizeMiB int64) (string, error) {
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
	node.lvols[lvolID] = &lvolISCSI{}

	klog.V(5).Infof("volume created: %s", lvolID)
	return lvolID, nil
}

func (node *nodeISCSI) CreateSnapshot(lvolName, snapshotName string) (string, error) {
	snapshotID, err := node.client.snapshot(lvolName, snapshotName)
	if err != nil {
		return "", err
	}

	klog.V(5).Infof("snapshot created: %s", snapshotID)
	return snapshotID, nil
}

func (node *nodeISCSI) DeleteVolume(lvolID string) error {
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

// PublishVolume exports a volume through ISCSI target
func (node *nodeISCSI) PublishVolume(lvolID string) error {
	var err error

	node.mtx.Lock()
	lvol, exists := node.lvols[lvolID]
	node.mtx.Unlock()

	if !exists {
		return ErrVolumeDeleted
	}
	if lvol.published {
		return ErrVolumePublished
	}

	err = node.createPortalGroup()
	if err != nil {
		return err
	}

	err = node.createInitiatorGroup()
	if err != nil {
		return err
	}
	// lvolID is unique and can be used as the target name
	var targetName = lvolID
	err = node.iscsiCreateTargetNode(targetName, lvolID)
	if err != nil {
		return err
	}

	node.lvols[lvolID].published = true
	return nil
}

func (node *nodeISCSI) createPortalGroup() error {
	err := node.iscsiGetPortalGroups()
	if err == nil {
		return nil // port group already exists
	}

	err = node.iscsiCreatePortalGroup()
	if err == nil {
		return nil // creation succeeds
	}
	// we may fail due to concurrent calls, check portal group availability again
	return node.iscsiGetPortalGroups()
}

func (node *nodeISCSI) createInitiatorGroup() error {
	err := node.iscsiGetInitiatorGroups()
	if err == nil {
		return nil
	}

	err = node.iscsiCreateInitiatorGroup([]string{"ANY"}, []string{"ANY"})
	if err == nil {
		return nil
	}

	return node.iscsiGetInitiatorGroups()
}

func (node *nodeISCSI) UnpublishVolume(lvolID string) error {
	var err error
	node.mtx.Lock()
	lvol, exists := node.lvols[lvolID]
	node.mtx.Unlock()

	if !exists {
		return ErrVolumeDeleted
	}
	if !lvol.published {
		return ErrVolumeUnpublished
	}

	err = node.iscsiDeleteTargetNode(lvolID)
	if err != nil {
		return err
	}

	lvol.reset()
	klog.V(5).Infof("volume unpublished: %s", lvolID)
	return nil
}

// Add a portal group
func (node *nodeISCSI) iscsiCreatePortalGroup() error {
	type Portals struct {
		Host string `json:"host"`
		Port string `json:"port"`
	}
	params := struct {
		Portals []Portals `json:"portals"`
		Tag     int       `json:"tag"`
	}{
		Portals: []Portals{{node.targetAddr, node.targetPort}},
		Tag:     numberPortalGroupTag,
	}
	var result bool
	err := node.client.call("iscsi_create_portal_group", &params, &result)
	if err != nil {
		return err
	}
	if !result {
		return fmt.Errorf("create iscsi portal group failure")
	}
	return nil
}

// Add an initiator group
func (node *nodeISCSI) iscsiCreateInitiatorGroup(initiators, netmasks []string) error {
	params := struct {
		Initiators []string `json:"initiators"`
		Tag        int      `json:"tag"`
		Netmasks   []string `json:"netmasks"`
	}{
		Initiators: initiators,
		Tag:        numberInitiatorGroupTag,
		Netmasks:   netmasks,
	}
	var result bool
	err := node.client.call("iscsi_create_initiator_group", &params, &result)
	if err != nil {
		return err
	}
	if !result {
		return fmt.Errorf("create iscsi initiator group failure")
	}
	return nil
}

// Add an iSCSI target node
func (node *nodeISCSI) iscsiCreateTargetNode(targetName, bdevName string) error {
	type Luns struct {
		LunID    int    `json:"lun_id"`
		BdevName string `json:"bdev_name"`
	}
	type PgIgMaps struct {
		IgTag int `json:"ig_tag"`
		PgTag int `json:"pg_tag"`
	}
	params := struct {
		Luns        []Luns     `json:"luns"`
		Name        string     `json:"name"`
		AliasName   string     `json:"alias_name"`
		PgIgMaps    []PgIgMaps `json:"pg_ig_maps"`
		DisableChap bool       `json:"disable_chap"`
		QueueDepth  int        `json:"queue_depth"`
	}{
		Luns:        []Luns{{0, bdevName}},
		Name:        targetName,
		AliasName:   "iscsi-" + bdevName,
		PgIgMaps:    []PgIgMaps{{numberPortalGroupTag, numberInitiatorGroupTag}},
		DisableChap: true,
		QueueDepth:  targetQueueDepth,
	}
	var result bool
	err := node.client.call("iscsi_create_target_node", &params, &result)
	if err != nil {
		return err
	}
	if !result {
		return fmt.Errorf("create iscsi target node failure")
	}
	return nil
}

// Delete an iSCSI target node
func (node *nodeISCSI) iscsiDeleteTargetNode(targetName string) error {
	params := struct {
		Name string `json:"name"`
	}{
		Name: iqnPrefixName + targetName,
	}
	var result bool
	err := node.client.call("iscsi_delete_target_node", &params, &result)
	if err != nil {
		return err
	}
	if !result {
		return fmt.Errorf("delete iscsi target node failure")
	}
	return nil
}

// Check if portal group is available
func (node *nodeISCSI) iscsiGetPortalGroups() error {
	var results []struct {
		Tag int `json:"tag"`
	}
	err := node.client.call("iscsi_get_portal_groups", nil, &results)
	if err != nil {
		return err
	}
	for _, value := range results {
		if value.Tag == numberPortalGroupTag {
			return nil
		}
	}
	return fmt.Errorf("port group not available")
}

func (node *nodeISCSI) iscsiGetInitiatorGroups() error {
	var results []struct {
		Tag int `json:"tag"`
	}
	err := node.client.call("iscsi_get_initiator_groups", nil, &results)
	if err != nil {
		return err
	}
	for _, value := range results {
		if value.Tag == numberInitiatorGroupTag {
			return nil
		}
	}
	return fmt.Errorf("initiator group not available")
}
