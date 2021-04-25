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

// NOTE: This test requires spdk target and jsonrpc http proxy run on localhost
// - start spdk target server
//   $ spdk/app/spdk_tgt/spdk_tgt
// - create test bdev and volume store
//   $ spdk/scripts/rpc.py bdev_malloc_create -b Malloc0 1024 4096
//   $ spdk/scripts/rpc.py bdev_lvol_create_lvstore Malloc0 lvs0
// - start jsonrpc http proxy
//   $ spdk/scripts/rpc_http_proxy.py 127.0.0.1 9009 spdkcsiuser spdkcsipass

package util

import (
	"fmt"
	"testing"
)

const (
	rpcURL  = "http://127.0.0.1:9009"
	rpcUser = "spdkcsiuser"
	rpcPass = "spdkcsipass"
	trAddr  = "127.0.0.1"
)

func TestNVMeTCP(t *testing.T) {
	testNVMeoF("nvme-tcp", t)
}

func testNVMeoF(trType string, t *testing.T) {
	nodeIx, err := NewSpdkNode(rpcURL, rpcUser, rpcPass, trType, trAddr)
	if err != nil {
		t.Fatalf("NewSpdkNode: %s", err)
	}

	node, ok := nodeIx.(*nodeNVMf)
	if !ok {
		t.Fatal("cannot cast to nodeNVMf")
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
	err = validateVolumeCreated(node, lvolID)
	if err != nil {
		t.Fatalf("validateVolumeCreated: %s", err)
	}

	err = node.PublishVolume(lvolID)
	if err != nil {
		t.Fatalf("PublishVolume: %s", err)
	}

	nqn := node.lvols[lvolID].nqn
	nsID := node.lvols[lvolID].nsID

	err = validateVolumePublished(node, nqn, nsID)
	if err != nil {
		t.Fatalf("validateVolumePublished: %s", err)
	}

	snapshotName := "snapshot-pvc"
	var snapshotID string
	snapshotID, err = node.CreateSnapshot(lvolID, snapshotName)
	if err != nil {
		t.Fatalf("CreateSnapshot: %s", err)
	}
	err = validateVolumeCreated(node, snapshotID)
	if err != nil {
		t.Fatalf("validateCreateSnapshot: %s", err)
	}

	err = node.DeleteVolume(snapshotID)
	if err != nil {
		t.Fatalf("DeleteSnapshot: %s", err)
	}
	err = validateVolumeDeleted(node, snapshotID)
	if err != nil {
		t.Fatalf("validateSnapshotDeleted: %s", err)
	}

	err = node.UnpublishVolume(lvolID)
	if err != nil {
		t.Fatalf("UnpublishVolume: %s", err)
	}
	err = validateVolumeUnpublished(node, nqn, nsID)
	if err != nil {
		t.Fatalf("validateVolumeUnpublished: %s", err)
	}

	err = node.DeleteVolume(lvolID)
	if err != nil {
		t.Fatalf("DeleteVolume: %s", err)
	}
	err = validateVolumeDeleted(node, lvolID)
	if err != nil {
		t.Fatalf("validateVolumeDeleted: %s", err)
	}
}

func validateVolumeCreated(node *nodeNVMf, lvolID string) error {
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

func validateVolumePublished(node *nodeNVMf, nqn string, nsID int) error {
	type namespace struct {
		NsID int `json:"nsid"`
	}

	var results []struct {
		Nqn         string      `json:"nqn"`
		ModelNumber string      `json:"model_number"`
		Namespaces  []namespace `json:"namespaces"`
	}

	err := node.client.call("nvmf_get_subsystems", nil, &results)
	if err != nil {
		return err
	}

	for i := range results {
		result := &results[i]
		if result.Nqn == nqn {
			if len(result.Namespaces) != 1 {
				return fmt.Errorf("#name_spaces != 1")
			}
			if result.Namespaces[0].NsID != nsID {
				return fmt.Errorf("nsid mismatch")
			}
			return nil
		}
	}

	return fmt.Errorf("nqn not found: %s", nqn)
}

func validateVolumeDeleted(node *nodeNVMf, lvolID string) error {
	if validateVolumeCreated(node, lvolID) == nil {
		return fmt.Errorf("volume not deleted")
	}
	return nil
}

func validateVolumeUnpublished(node *nodeNVMf, nqn string, nsID int) error {
	if validateVolumePublished(node, nqn, nsID) == nil {
		return fmt.Errorf("volume not unpublished")
	}
	return nil
}
