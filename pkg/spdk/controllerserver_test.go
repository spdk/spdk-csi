package spdk

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"

	csicommon "github.com/spdk/spdk-csi/pkg/csi-common"
	"github.com/spdk/spdk-csi/pkg/util"
)

func TestNvmeofVolume(t *testing.T) {
	testVolume("nvme-tcp", t)
}

func TestNvmeofIdempotency(t *testing.T) {
	testIdempotency("nvme-tcp", t)
}

func TestNvmeofConcurrency(t *testing.T) {
	testConcurrency("nvme-tcp", t)
}

func TestIscsiVolume(t *testing.T) {
	testVolume("iscsi", t)
}

func TestIscsiIdempotency(t *testing.T) {
	testIdempotency("iscsi", t)
}

func TestIscsiConcurrency(t *testing.T) {
	testConcurrency("iscsi", t)
}

func testVolume(targetType string, t *testing.T) {
	cs, lvss, err := createTestController(targetType)
	if err != nil {
		t.Fatal(err)
	}

	const volumeName = "test-volume"
	const volumeSize = 256 * 1024 * 1024

	// create volume#1
	volumeID1, err := createTestVolume(cs, volumeName, volumeSize)
	if err != nil {
		t.Fatal(err)
	}
	// delete volume#1
	err = deleteTestVolume(cs, volumeID1)
	if err != nil {
		t.Fatal(err)
	}

	// create volume#2 with same name
	volumeID2, err := createTestVolume(cs, volumeName, volumeSize)
	if err != nil {
		t.Fatal(err)
	}
	// delete volume#2
	err = deleteTestVolume(cs, volumeID2)
	if err != nil {
		t.Fatal(err)
	}

	// make sure volumeID1 != volumeID2
	if volumeID1 == volumeID2 {
		t.Fatal("volume id should be different")
	}

	// make sure we didn't leave any garbage in lvstores
	if !verifyLVSS(cs, lvss) {
		t.Fatal("lvstore status doesn't match")
	}
}

func testIdempotency(targetType string, t *testing.T) {
	cs, lvss, err := createTestController(targetType)
	if err != nil {
		t.Fatal(err)
	}

	const volumeName = "test-volume-idem"
	const requestCount = 50
	const volumeSize = 256 * 1024 * 1024

	volumeID, err := createSameVolumeInParallel(cs, volumeName, requestCount, volumeSize)
	if err != nil {
		t.Fatal(err)
	}
	err = deleteSameVolumeInParallel(cs, volumeID, requestCount)
	if err != nil {
		t.Fatal(err)
	}

	if !verifyLVSS(cs, lvss) {
		t.Fatal("lvstore status doesn't match")
	}
}

func testConcurrency(targetType string, t *testing.T) {
	cs, lvss, err := createTestController(targetType)
	if err != nil {
		t.Fatal(err)
	}

	// count * size cannot exceed storage capacity
	const volumeNamePrefix = "test-volume-con-"
	const volumeCount = 20
	const volumeSize = 16 * 1024 * 1024

	// create/delete multiple volumes in parallel
	var wg sync.WaitGroup
	var errCount int32
	for i := 0; i < volumeCount; i++ {
		volumeName := fmt.Sprintf("%s%d", volumeNamePrefix, i)
		wg.Add(1)
		go func(volumeNameLocal string) {
			defer wg.Done()

			volumeIDLocal, errLocal := createTestVolume(cs, volumeNameLocal, volumeSize)
			if errLocal != nil {
				t.Logf("createTestVolume failed: %s", errLocal)
				atomic.AddInt32(&errCount, 1)
			}

			errLocal = deleteTestVolume(cs, volumeIDLocal)
			if errLocal != nil {
				t.Logf("deleteTestVolume failed: %s", errLocal)
				atomic.AddInt32(&errCount, 1)
			}
		}(volumeName)
	}
	wg.Wait()
	if errCount != 0 {
		t.Fatal("concurrency test failed")
	}

	if !verifyLVSS(cs, lvss) {
		t.Fatal("lvstore status doesn't match")
	}
}

func createTestController(targetType string) (cs *controllerServer, lvss [][]util.LvStore, err error) {
	err = createConfigFiles(targetType)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		os.Remove(os.Getenv("SPDKCSI_CONFIG"))
		os.Remove(os.Getenv("SPDKCSI_SECRET"))
	}()

	cd := csicommon.NewCSIDriver("test-driver", "test-version", "test-node")
	cs, err = newControllerServer(cd)
	if err != nil {
		return nil, nil, err
	}

	lvss, err = getLVSS(cs)
	if err != nil {
		return nil, nil, err
	}

	return cs, lvss, nil
}

func createConfigFiles(targetType string) error {
	configFile, err := ioutil.TempFile("", "spdkcsi-config*.json")
	if err != nil {
		return err
	}
	defer configFile.Close()
	var config string
	switch targetType {
	case "nvme-tcp":
		config = `
    {
      "nodes": [
        {
          "name": "localhost",
          "rpcURL": "http://127.0.0.1:9009",
          "targetType": "nvme-tcp",
          "targetAddr": "127.0.0.1"
        }
      ]
	}`
	case "iscsi":
		config = `
    {
      "nodes": [
        {
          "name": "localhost",
          "rpcURL": "http://127.0.0.1:9009",
          "targetType": "iscsi",
          "targetAddr": "127.0.0.1"
        }
      ]
    }`
	}
	_, err = configFile.Write([]byte(config))
	if err != nil {
		os.Remove(configFile.Name())
		return err
	}
	os.Setenv("SPDKCSI_CONFIG", configFile.Name())

	secretFile, err := ioutil.TempFile("", "spdkcsi-secret*.json")
	if err != nil {
		os.Remove(configFile.Name())
		return err
	}
	defer secretFile.Close()
	// nolint:gosec // only for test
	secret := `
    {
      "rpcTokens": [
        {
          "name": "localhost",
          "username": "spdkcsiuser",
          "password": "spdkcsipass"
        }
      ]
	}`
	_, err = secretFile.Write([]byte(secret))
	if err != nil {
		os.Remove(configFile.Name())
		os.Remove(secretFile.Name())
		return err
	}
	os.Setenv("SPDKCSI_SECRET", secretFile.Name())

	return nil
}

func createTestVolume(cs *controllerServer, name string, size int64) (string, error) {
	reqCreate := csi.CreateVolumeRequest{
		Name:          name,
		CapacityRange: &csi.CapacityRange{RequiredBytes: size},
	}

	resp, err := cs.CreateVolume(context.TODO(), &reqCreate)
	if err != nil {
		return "", err
	}

	volumeID := resp.GetVolume().GetVolumeId()
	if volumeID == "" {
		return "", fmt.Errorf("empty volume id")
	}

	return volumeID, nil
}

func deleteTestVolume(cs *controllerServer, volumeID string) error {
	reqDelete := csi.DeleteVolumeRequest{VolumeId: volumeID}
	_, err := cs.DeleteVolume(context.TODO(), &reqDelete)
	return err
}

func createSameVolumeInParallel(cs *controllerServer, name string, count int, size int64) (string, error) {
	var wg sync.WaitGroup
	var errCount int32

	// issue multiple create requests to create *same* volume in parallel
	volumeID := make([]string, count)
	reqCreate := csi.CreateVolumeRequest{
		Name:          name,
		CapacityRange: &csi.CapacityRange{RequiredBytes: size},
	}
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int, wg *sync.WaitGroup) {
			defer wg.Done()
			for {
				resp, errLocal := cs.CreateVolume(context.TODO(), &reqCreate)
				if errLocal == errVolumeInCreation {
					continue
				}
				if errLocal != nil {
					atomic.AddInt32(&errCount, 1)
				}
				volumeID[i] = resp.GetVolume().GetVolumeId()
				break
			}
		}(i, &wg)
	}
	wg.Wait()

	if atomic.LoadInt32(&errCount) != 0 {
		return "", fmt.Errorf("some cs.CreateVolume failed")
	}

	// verify all returned volume ids are not empty and identical
	if volumeID[0] == "" {
		return "", fmt.Errorf("empty volume id")
	}
	for i := 1; i < count; i++ {
		if volumeID[i] != volumeID[0] {
			return "", fmt.Errorf("volume id mismatch")
		}
	}

	return volumeID[0], nil
}

func deleteSameVolumeInParallel(cs *controllerServer, volumeID string, count int) error {
	var wg sync.WaitGroup
	var errCount int32

	// issue delete requests to *same* volume in parallel
	reqDelete := csi.DeleteVolumeRequest{VolumeId: volumeID}
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			_, err := cs.DeleteVolume(context.TODO(), &reqDelete)
			if err != nil {
				atomic.AddInt32(&errCount, 1)
			}
		}(&wg)
	}
	wg.Wait()

	if atomic.LoadInt32(&errCount) != 0 {
		return fmt.Errorf("some cs.DeleteVolume failed")
	}
	return nil
}

func getLVSS(cs *controllerServer) ([][]util.LvStore, error) {
	var lvss [][]util.LvStore
	for _, spdkNode := range cs.spdkNodes {
		lvs, err := spdkNode.LvStores()
		if err != nil {
			return nil, err
		}
		lvss = append(lvss, lvs)
	}
	return lvss, nil
}

func verifyLVSS(cs *controllerServer, lvss [][]util.LvStore) bool {
	lvssNow, err := getLVSS(cs)
	if err != nil {
		return false
	}

	// deep compare lvss and lvssNow
	if len(lvss) != len(lvssNow) {
		return false
	}
	for i := 0; i < len(lvss); i++ {
		lvs := lvss[i]
		lvsNow := lvssNow[i]
		if len(lvs) != len(lvsNow) {
			return false
		}
		for j := 0; j < len(lvs); j++ {
			if lvs[j] != lvsNow[j] {
				return false
			}
		}
	}
	return true
}
