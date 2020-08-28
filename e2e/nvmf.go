package e2e

import (
	"time"

	"github.com/onsi/ginkgo"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	nvmeofConfigMapData = `{
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

var _ = ginkgo.Describe("SPDKCSI-NVMF", func() {
	f := framework.NewDefaultFramework("spdkcsi")
	ginkgo.BeforeEach(func() {
		deployConfigs(nvmeofConfigMapData)
		deployCsi()
	})

	ginkgo.AfterEach(func() {
		deleteCsi()
		deleteConfigs()
	})

	ginkgo.Context("Test SPDK CSI NVMF", func() {
		ginkgo.It("Test SPDK CSI NVMF", func() {
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

			ginkgo.By("checking test pod is running", func() {
				deployTestPod()
				defer deleteTestPod()
				err := waitForTestPodReady(f.ClientSet, 5*time.Minute)
				if err != nil {
					ginkgo.Fail(err.Error())
				}
			})
		})
	})
})
