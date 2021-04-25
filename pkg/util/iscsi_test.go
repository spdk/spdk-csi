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
	"testing"
)

const (
	rpcURLISCSI  = "http://127.0.0.1:9009"
	rpcUserISCSI = "spdkcsiuser"
	rpcPassISCSI = "spdkcsipass"
	trAddrISCSI  = "127.0.0.1"
)

func TestISCSI(t *testing.T) {
	nodeIx, err := NewSpdkNode(rpcURLISCSI, rpcUserISCSI, rpcPassISCSI, "ISCSI", trAddrISCSI)
	if err != nil {
		t.Fatalf("NewSpdkNode: %s", err)
	}
	node, ok := nodeIx.(*nodeISCSI)
	if !ok {
		t.Fatal("cannot cast to nodeISCSI")
	}

	lvs, err := node.LvStores()
	if err != nil {
		t.Fatalf("LvStores: %s", err)
	}
	if len(lvs) == 0 {
		t.Fatal("No logical volume store")
	}
	if lvs[0].FreeSizeMiB == 0 {
		t.Fatalf("No free space: %s", lvs[0].Name)
	}

	lvolID, err := node.CreateVolume(lvs[0].Name, lvs[0].FreeSizeMiB)
	if err != nil {
		t.Fatalf("CreateVolume: %s", err)
	}
	err = iscsiValidateVolumeCreated(node, lvolID)
	if err != nil {
		t.Fatalf("validateVolumeCreated: %s", err)
	}

	err = node.PublishVolume(lvolID)
	if err != nil {
		t.Fatalf("PublishVolume: %s", err)
	}

	err = iscsiValidateVolumePublished(node, lvolID)

	if err != nil {
		t.Fatalf("iscsiValidateVolumePublished: %s", err)
	}

	snapshotName := "snapshot-pvc"
	var snapshotID string
	snapshotID, err = node.CreateSnapshot(lvolID, snapshotName)
	if err != nil {
		t.Fatalf("CreateSnapshot: %s", err)
	}
	err = iscsiValidateVolumeCreated(node, snapshotID)
	if err != nil {
		t.Fatalf("validateCreateSnapshot: %s", err)
	}

	err = node.DeleteVolume(snapshotID)
	if err != nil {
		t.Fatalf("DeleteSnapshot: %s", err)
	}
	err = iscsiValidateVolumeDeleted(node, snapshotID)
	if err != nil {
		t.Fatalf("validateSnapshotDeleted: %s", err)
	}

	err = node.UnpublishVolume(lvolID)
	if err != nil {
		t.Fatalf("UnpublishVolume: %s", err)
	}

	err = node.DeleteVolume(lvolID)
	if err != nil {
		t.Fatalf("DeleteVolume: %s", err)
	}

	err = iscsiValidateVolumeDeleted(node, lvolID)
	if err != nil {
		t.Fatalf("validateVolumeDeleted: %s", err)
	}
}

func iscsiValidateVolumeDeleted(node *nodeISCSI, lvolID string) error {
	if iscsiValidateVolumeCreated(node, lvolID) == nil {
		return fmt.Errorf("volume not deleted")
	}
	return nil
}

func iscsiValidateVolumeCreated(node *nodeISCSI, lvolID string) error {
	params := struct {
		Name string `json:"name"`
	}{
		Name: lvolID,
	}

	var result []struct {
		BlockSize int64 `json:"block_size"`
		NumBlocks int64 `json:"num_blocks"`
	}

	err := node.client.call("bdev_get_bdevs", &params, &result)
	if err != nil {
		return err
	}

	if len(result) == 0 {
		return fmt.Errorf("lvol not found: %s", lvolID)
	}

	return nil
}

func iscsiValidateVolumePublished(node *nodeISCSI, lvolID string) error {
	iqn := "iqn.2016-06.io.spdk:" + lvolID
	var result []struct {
		Name string `json:"name"`
	}

	err := node.client.call("iscsi_get_target_nodes", nil, &result)
	if err != nil {
		return err
	}
	for _, value := range result {
		if value.Name == iqn {
			return nil
		}
	}

	return fmt.Errorf("iqn not found: %s", iqn)
}
