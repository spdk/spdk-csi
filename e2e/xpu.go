package e2e

import (
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	xpuControllerConfigMapData = `{
  "nodes": [
    {
      "name": "localhost",
      "rpcURL": "http://127.0.0.1:9009",
      "targetType": "nvme-tcp",
      "targetAddr": "127.0.0.1"
    }
  ]
}`
)

var _ = ginkgo.Describe("SPDKCSI-XPU", func() {
	f := framework.NewDefaultFramework("spdkcsi")

	commonTests := func() {
		ginkgo.By("checking controller statefulset is running", func() {
			err := waitForControllerReady(f.ClientSet, 4*time.Minute)
			if err != nil {
				ginkgo.Fail(err.Error())
			}
		})

		ginkgo.By("checking node daemonset is running", func() {
			err := waitForNodeServerReady(f.ClientSet, 2*time.Minute)
			if err != nil {
				ginkgo.Fail(err.Error())
			}
		})

		ginkgo.By("create multiple pvcs and a pod with multiple pvcs attached, and check data persistence after the pod is removed and recreated", func() {
			deployMultiPvcs()
			deployTestPodWithMultiPvcs()
			defer func() {
				deleteMultiPvcsAndTestPodWithMultiPvcs()
				if err := waitForTestPodGone(f.ClientSet); err != nil {
					ginkgo.Fail(err.Error())
				}
				for _, pvcName := range []string{"spdkcsi-pvc1", "spdkcsi-pvc2", "spdkcsi-pvc3"} {
					if err := waitForPvcGone(f.ClientSet, pvcName); err != nil {
						ginkgo.Fail(err.Error())
					}
				}
			}()
			err := waitForTestPodReady(f.ClientSet, 5*time.Minute)
			if err != nil {
				ginkgo.Fail(err.Error())
			}

			err = checkDataPersistForMultiPvcs(f)
			if err != nil {
				ginkgo.Fail(err.Error())
			}
		})
	}

	ginkgo.Describe("Test SPDK CSI SMA NVMF-TCP", func() {
		ginkgo.BeforeEach(func() {
			deployConfigs(xpuControllerConfigMapData)
			deploySmaNvmfConfig()
			deployCsi()
		})

		ginkgo.AfterEach(func() {
			deleteCsi()
			deleteSmaNvmfConfig()
			deleteConfigs()
		})

		ginkgo.It("SPDKCSI-SMA-NVMF-TCP", func() {
			commonTests()

			ginkgo.By("log verification for SMA-NVMF-TCP workflow", func() {
				expLogList := []string{
					"connected to SMA server 127.0.0.1:5114 with TargetType as xpu-sma-nvmftcp",
					"SMA.CreateDevice",
					"SMA.AttachVolume",
					"SMA.DetachVolume",
					"SMA.DeleteDevice",
				}
				err := verifyNodeServerLog(expLogList)
				if err != nil {
					ginkgo.Fail(err.Error())
				}
			})
		})
	})

	ginkgo.Describe("Test SPDK CSI SMA NVME", func() {
		ginkgo.BeforeEach(func() {
			deployConfigs(xpuControllerConfigMapData)
			deploySmaNvmeConfig()
			deployCsi()
		})

		ginkgo.AfterEach(func() {
			deleteCsi()
			deleteSmaNvmeConfig()
			deleteConfigs()
		})

		ginkgo.It("SPDKCSI-SMA-NVME", func() {
			if !runXPU {
				ginkgo.Skip("skip SPDKCSI-SMA-NVME test as runXPU is false")
			}

			commonTests()

			ginkgo.By("log verification for SMA-NVME workflow", func() {
				expLogList := []string{
					"connected to SMA server 127.0.0.1:5114 with TargetType as xpu-sma-nvme",
					"SMA.CreateDevice",
					"SMA.AttachVolume",
					"SMA.DetachVolume",
				}
				err := verifyNodeServerLog(expLogList)
				if err != nil {
					ginkgo.Fail(err.Error())
				}
			})
		})
	})
})
