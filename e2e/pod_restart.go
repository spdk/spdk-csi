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

package e2e

import (
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = ginkgo.Describe("SPDKCSI-DRIVER-RESTART", func() {
	f := framework.NewDefaultFramework("spdkcsi")
	ginkgo.BeforeEach(func() {
		deployConfigs(nvmeofConfigMapData)
		deployCsi()
	})
	ginkgo.AfterEach(func() {
		deleteCsi()
		deleteConfigs()
	})
	ginkgo.Context("Test SPDK-CSI Restart", func() {
		ginkgo.It("Test SPDK-CSI Restart", func() {
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
			ginkgo.By("create a PVC and bind it to a pod", func() {
				deployPVC()
				deployTestPod()
				err := waitForTestPodReady(f.ClientSet, 5*time.Minute)
				if err != nil {
					deletePVCAndTestPod()
					ginkgo.Fail(err.Error())
				}
			})
			ginkgo.By("restart csi driver", func() {
				rolloutNodeServer()
				rolloutControllerServer()
				err := waitForNodeServerReady(f.ClientSet, 3*time.Minute)
				if err != nil {
					ginkgo.Fail(err.Error())
				}
				err = waitForControllerReady(f.ClientSet, 4*time.Minute)
				if err != nil {
					ginkgo.Fail(err.Error())
				}
			})
			ginkgo.By("delete test pod", func() {
				// Under normal circumstances, the pod will be immediately deleted.
				// When the nodeserver encounters an error, the kubelet will continuously retry,
				// resulting in the operation timing out.
				err := deleteTestPodWithTimeout(120 * time.Second)
				defer deletePVC()
				if err != nil {
					deleteTestPodForce()
					ginkgo.Fail(err.Error())
				}
			})
		})
	})
})
