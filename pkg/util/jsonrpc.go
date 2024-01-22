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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/klog"
)

// SpdkNode defines interface for SPDK storage node
//
//   - Info returns node info(rpc url) for debugging purpose
//   - LvStores returns available volume stores(name, size, etc) on that node.
//   - VolumeInfo returns a string map to be passed to client node. Client node
//     needs these info to mount the target. E.g, target IP, service port, nqn.
//   - Create/Delete/Publish/UnpublishVolume per CSI controller service spec.
//
// NOTE: concurrency, idempotency, message ordering
//
// In below text, "implementation" refers to the code implements this interface,
// and "caller" is the code uses the implementation.
//
// Concurrency requirements for implementation and caller:
//   - Implementation should make sure CreateVolume is thread
//     safe. Caller is free to request creating multiple volumes in
//     same volume store concurrently, no data race should happen.
//   - Implementation should make sure
//     PublishVolume/UnpublishVolume/DeleteVolume for *different
//     volumes* thread safe. Caller may issue these requests to
//     *different volumes", in same volume store or not, concurrently.
//   - PublishVolume/UnpublishVolume/DeleteVolume for *same volume* is
//     not thread safe, concurrent access may lead to data
//     race. Caller must serialize these calls to *same volume*,
//     possibly by mutex or message queue per volume.
//   - Implementation should make sure LvStores and VolumeInfo are
//     thread safe, but it doesn't lock the returned resources. It
//     means caller should adopt optimistic concurrency control and
//     retry on specific failures.  E.g, caller calls LvStores and
//     finds a volume store with enough free space, it calls
//     CreateVolume but fails with "not enough space" because another
//     caller may issue similar request at same time. The failed
//     caller may redo above steps(call LvStores, pick volume store,
//     CreateVolume) under this condition, or it can simply fail.
//
// Idempotent requirements for implementation:
// Per CSI spec, it's possible that same request been sent multiple times due to
// issues such as a temporary network failure. Implementation should have basic
// logic to deal with idempotency.
// E.g, ignore publishing an already published volume.
//
// Out of order messages handling for implementation:
// Out of order message may happen in kubernetes CSI framework. E.g, unpublish
// an already deleted volume. The baseline is there should be no code crash or
// data corruption under these conditions. Implementation may try to detect and
// report errors if possible.
type SpdkNode interface {
	Info() string
	LvStores() ([]LvStore, error)
	VolumeInfo(lvolID string) (map[string]string, error)
	CreateVolume(lvolName, lvsName string, sizeMiB int64) (string, error)
	GetVolume(lvolName, lvsName string) (string, error)
	DeleteVolume(lvolID string) error
	PublishVolume(lvolID string) error
	UnpublishVolume(lvolID string) error
	CreateSnapshot(lvolName, snapshotName string) (string, error)
	DeleteSnapshot(snapshotID string) error
}

// logical volume store
type LvStore struct {
	Name         string
	UUID         string
	TotalSizeMiB int64
	FreeSizeMiB  int64
}

// BDev SPDK block device
type BDev struct {
	Name           string `json:"name"`
	UUID           string `json:"uuid"`
	BlockSize      int64  `json:"block_size"`
	NumBlocks      int64  `json:"num_blocks"`
	DriverSpecific *struct {
		Lvol struct {
			LvolStoreUUID string `json:"lvol_store_uuid"`
		} `json:"lvol"`
	} `json:"driver_specific,omitempty"`
}

// errors deserve special care
var (
	// json response errors: errors.New("json: tag-string")
	// matches if "tag-string" founded in json error string
	ErrJSONNoSpaceLeft   = errors.New("json: No space left")
	ErrJSONNoSuchDevice  = errors.New("json: No such device")
	ErrInvalidParameters = errors.New("json: Invalid parameters")

	// internal errors
	ErrVolumeDeleted     = errors.New("volume deleted")
	ErrVolumePublished   = errors.New("volume already published")
	ErrVolumeUnpublished = errors.New("volume not published")
)

// jsonrpc http proxy
type rpcClient struct {
	ClusterID     string
	ClusterIP     string
	ClusterSecret string
	httpClient    *http.Client
	// rpcID          int32 // json request message ID, auto incremented
}

// func NewSpdkNode(rpcURL, rpcUser, rpcPass, targetType, targetAddr string) (SpdkNode, error) {
// 	client := rpcClient{
// 		rpcURL:     rpcURL,
// 		rpcUser:    rpcUser,
// 		rpcPass:    rpcPass,
// 		httpClient: &http.Client{Timeout: cfgRPCTimeoutSeconds * time.Second},
// 	}

// 	switch strings.ToLower(targetType) {
// 	case "nvme-rdma":
// 		return newNVMf(&client, "RDMA", targetAddr), nil
// 	case "nvme-tcp":
// 		return newNVMf(&client, "TCP", targetAddr), nil
// 	case "iscsi":
// 		return newISCSI(&client, targetAddr), nil
// 	default:
// 		return nil, fmt.Errorf("unknown transport: %s", targetType)
// 	}
// }

func (client *rpcClient) info() string {
	return client.ClusterID
}

type CSIPoolsResp struct {
	FreeClusters  int64  `json:"free_clusters"`
	ClusterSize   int64  `json:"cluster_size"`
	TotalClusters int64  `json:"total_data_clusters"`
	Name          string `json:"name"`
	UUID          string `json:"uuid"`
}

func (client *rpcClient) lvStores() ([]LvStore, error) {
	var result []CSIPoolsResp

	out, err := client.callSBCLI("GET", "csi/get_pools", nil)
	if err != nil {
		return nil, err
	}

	result, ok := out.([]CSIPoolsResp)
	if !ok {
		return nil, fmt.Errorf("failed to convert the response to CSIPoolsResp type. Interface: %v", out)
	}

	lvs := make([]LvStore, len(result))
	for i := range result {
		r := &result[i]
		lvs[i].Name = r.Name
		lvs[i].UUID = r.UUID
		lvs[i].TotalSizeMiB = r.TotalClusters * r.ClusterSize / 1024 / 1024
		lvs[i].FreeSizeMiB = r.FreeClusters * r.ClusterSize / 1024 / 1024
	}

	return lvs, nil
}

// createVolume create a logical volume with simplyblock storage
func (client *rpcClient) createVolume(params *CreateLVolData) (string, error) {
	var lvolID string
	klog.V(5).Info("params", params)
	out, err := client.callSBCLI("POST", "/lvol", &params)
	if err != nil {
		if errorMatches(err, ErrJSONNoSpaceLeft) {
			err = ErrJSONNoSpaceLeft // may happen in concurrency
		}
		return "", err
	}

	lvolID, ok := out.(string)
	if !ok {
		return "", fmt.Errorf("failed to convert the response to string type. Interface: %v", out)
	}
	return lvolID, err
}

// get a volume and return a BDev,, lvsName/lvolName
func (client *rpcClient) getVolume(lvolID string) (*BDev, error) {
	var result []BDev
	out, err := client.callSBCLI("GET", fmt.Sprintf("csi/get_volume_info/%s", lvolID), nil)
	if err != nil {
		if errorMatches(err, ErrJSONNoSuchDevice) {
			err = ErrJSONNoSuchDevice
		}
		return nil, err
	}

	result, ok := out.([]BDev)
	if !ok {
		return nil, fmt.Errorf("failed to convert the response to []BDev type. Interface: %v", out)
	}
	return &result[0], err
}

type LvolConnectResp struct {
	Nqn     string `json:"nqn"`
	Port    int    `json:"port"`
	IP      string `json:"ip"`
	Connect string `json:"connect"`
}

type connectionInfo struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// get a volume and return a BDev
func (client *rpcClient) getVolumeInfo(lvolID string) (map[string]string, error) {
	var result []LvolConnectResp

	out, err := client.callSBCLI("GET",
		fmt.Sprintf("/lvol/connect/%s", lvolID), nil)
	if err != nil {
		klog.Error(err)
		if errorMatches(err, ErrJSONNoSuchDevice) {
			err = ErrJSONNoSuchDevice
		}
		return nil, err
	}

	byteData, err := json.Marshal(out)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	err = json.Unmarshal(byteData, &result)
	if err != nil {
		return nil, err
	}

	var connections []connectionInfo
	for _, r := range result {
		connections = append(connections, connectionInfo{IP: r.IP, Port: r.Port})
	}

	connectionsData, err := json.Marshal(connections)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return map[string]string{
		"name":        lvolID,
		"uuid":        lvolID,
		"nqn":         result[0].Nqn,
		"model":       lvolID,
		"targetType":  "tcp",
		"connections": string(connectionsData),
	}, nil
}

// func (client *rpcClient) isVolumeCreated(lvolID string) (bool, error) {
// 	_, err := client.getVolume(lvolID)
// 	if err != nil {
// 		if errors.Is(err, ErrJSONNoSuchDevice) {
// 			return false, nil
// 		}
// 		return false, err
// 	}
// 	return true, nil
// }

func (client *rpcClient) deleteVolume(lvolID string) error {
	_, err := client.callSBCLI("DELETE",
		fmt.Sprintf("csi/delete_lvol/%s", lvolID), nil)
	if errorMatches(err, ErrJSONNoSuchDevice) {
		err = ErrJSONNoSuchDevice // may happen in concurrency
	}

	return err
}

type ResizeVolReq struct {
	LvolID  string `json:"lvol_id"`
	NewSize int64  `json:"new_size"`
}

func (client *rpcClient) resizeVolume(lvolID string, newSize int64) (bool, error) {
	params := ResizeVolReq{
		LvolID:  lvolID,
		NewSize: newSize,
	}
	var result bool
	out, err := client.callSBCLI("POST", "csi/resize_lvol", &params)
	if err != nil {
		return false, err
	}
	result, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("failed to convert the response to bool type. Interface: %v", out)
	}
	return result, nil
}

type SnapshotResp struct {
	Name       string `json:"name"`
	UUID       string `json:"uuid"`
	Size       string `json:"size"`
	PoolName   string `json:"pool_name"`
	PoolID     string `json:"pool_id"`
	SourceUUID string `json:"source_uuid"`
	CreatedAt  string `json:"created_at"`
}

func (client *rpcClient) listSnapshots() ([]*SnapshotResp, error) {
	var results []*SnapshotResp

	out, err := client.callSBCLI("GET", "csi/list_snapshots", nil)
	if err != nil {
		return nil, err
	}
	results, ok := out.([]*SnapshotResp)
	if !ok {
		return nil, fmt.Errorf("failed to convert the response to []ResizeVolResp type. Interface: %v", out)
	}
	return results, nil
}

func (client *rpcClient) deleteSnapshot(snapshotID string) error {
	_, err := client.callSBCLI("DELETE",
		fmt.Sprintf("csi/delete_snapshot/%s", snapshotID), nil)

	if errorMatches(err, ErrJSONNoSuchDevice) {
		err = ErrJSONNoSuchDevice // may happen in concurrency
	}

	return err
}

func (client *rpcClient) snapshot(lvolID, snapShotName, poolName string) (string, error) {
	params := struct {
		LvolName     string `json:"lvol_id"`
		SnapShotName string `json:"snapshot_name"`
		PoolName     string `json:"pool_name"`
	}{
		LvolName:     lvolID,
		SnapShotName: snapShotName,
		PoolName:     poolName,
	}
	var snapshotID string
	out, err := client.callSBCLI("POST", "csi/create_snapshot", &params)
	snapshotID, ok := out.(string)
	if !ok {
		return "", fmt.Errorf("failed to convert the response to []ResizeVolResp type. Interface: %v", out)
	}
	return snapshotID, err
}

// getLvstore get lvstore name for specific lvol
// func (client *rpcClient) getLvstore(lvolID string) (string, error) {
// 	lvol, err := client.getVolume(lvolID)
// 	if err != nil {
// 		return "", err
// 	}
// 	if lvol.DriverSpecific == nil {
// 		return "", fmt.Errorf("no driver_specific for %s", lvolID)
// 	}
// 	lvstoreUUID := lvol.DriverSpecific.Lvol.LvolStoreUUID
// 	lvstores, err := client.lvStores()
// 	if err != nil {
// 		return "", err
// 	}
// 	for i := range lvstores {
// 		if lvstores[i].UUID == lvstoreUUID {
// 			return lvstores[i].Name, nil
// 		}
// 	}
// 	return "", fmt.Errorf("lvstore for %s not found", lvolID)
// }

// low level rpc request/response handling
// func (client *rpcClient) call(method string, args, result interface{}) error {
// 	type rpcRequest struct {
// 		Ver    string `json:"jsonrpc"`
// 		ID     int32  `json:"id"`
// 		Method string `json:"method"`
// 	}

// 	id := atomic.AddInt32(&client.rpcID, 1)
// 	request := rpcRequest{
// 		Ver:    "2.0",
// 		ID:     id,
// 		Method: method,
// 	}

// 	var data []byte
// 	var err error

// 	if args == nil {
// 		data, err = json.Marshal(request)
// 	} else {
// 		requestWithParams := struct {
// 			rpcRequest
// 			Params interface{} `json:"params"`
// 		}{
// 			request,
// 			args,
// 		}
// 		data, err = json.Marshal(requestWithParams)
// 	}
// 	if err != nil {
// 		return fmt.Errorf("%s: %w", method, err)
// 	}

// 	req, err := http.NewRequest(http.MethodPost, client.ClusterIP, bytes.NewReader(data))
// 	if err != nil {
// 		return fmt.Errorf("%s: %w", method, err)
// 	}

// 	req.Header.Set("Content-Type", "application/json")

// 	resp, err := client.httpClient.Do(req)
// 	if err != nil {
// 		return fmt.Errorf("%s: %w", method, err)
// 	}

// 	defer resp.Body.Close()
// 	if resp.StatusCode >= http.StatusBadRequest {
// 		return fmt.Errorf("%s: HTTP error code: %d", method, resp.StatusCode)
// 	}

// 	response := struct {
// 		ID    int32 `json:"id"`
// 		Error struct {
// 			Code    int    `json:"code"`
// 			Message string `json:"message"`
// 		} `json:"error"`
// 		Result interface{} `json:"result"`
// 	}{
// 		Result: result,
// 	}

// 	err = json.NewDecoder(resp.Body).Decode(&response)
// 	if err != nil {
// 		return fmt.Errorf("%s: %w", method, err)
// 	}
// 	if response.ID != id {
// 		return fmt.Errorf("%s: json response ID mismatch", method)
// 	}
// 	if response.Error.Code != 0 {
// 		return fmt.Errorf("%s: json response error: %s", method, response.Error.Message)
// 	}

// 	return nil
// }

type CreateVolResp struct {
	LVols []string `json:"lvols"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (client *rpcClient) callSBCLI(method, path string, args interface{}) (interface{}, error) {
	data := []byte(`{}`)
	var err error

	if args != nil {
		data, err = json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", method, err)
		}
	}

	requestURL := fmt.Sprintf("http://%s/%s", client.ClusterIP, path)
	req, err := http.NewRequest(method, requestURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}

	authHeader := fmt.Sprintf("%s %s", client.ClusterID, client.ClusterSecret)
	// klog.V(5).Infoln("requestURL", requestURL)
	// klog.V(5).Infoln("client.ClusterIP", client.ClusterIP)
	// klog.V(5).Infoln("path", path)
	// klog.V(5).Infoln("authHeader", authHeader)
	// klog.V(5).Infoln("body", string(data))

	req.Header.Add("Authorization", authHeader)
	req.Header.Add("cluster", client.ClusterID)
	req.Header.Add("secret", client.ClusterSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}

	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s: HTTP error code: %d", method, resp.StatusCode)
	}

	var response struct {
		Result  interface{} `json:"result"`
		Results interface{} `json:"results"`
		Error   Error       `json:"error"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("%s: HTTP error code: %d Error: %w", method, resp.StatusCode, err)
	}

	// klog.V(5).Info("response.Obj", response)

	if response.Error.Code > 0 {
		return nil, fmt.Errorf(response.Error.Message)
	}
	if response.Result != nil {
		return response.Result, nil
	}
	// klog.V(5).Info("response.Results", response.Results)
	return response.Results, nil
}

func errorMatches(errFull, errJSON error) bool {
	if errFull == nil {
		return false
	}
	strFull := strings.ToLower(errFull.Error())
	strJSON := strings.ToLower(errJSON.Error())
	strJSON = strings.TrimPrefix(strJSON, "json:")
	strJSON = strings.TrimSpace(strJSON)
	return strings.Contains(strFull, strJSON)
}
