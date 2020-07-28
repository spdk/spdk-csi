package e2e

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

const (
	nameSpace = "default"

	// deployment yaml files
	yamlDir            = "../deploy/kubernetes-1.17/"
	configMapPath      = yamlDir + "config-map.yaml"
	secretPath         = yamlDir + "secret.yaml"
	controllerRbacPath = yamlDir + "controller-rbac.yaml"
	nodeRbacPath       = yamlDir + "node-rbac.yaml"
	controllerPath     = yamlDir + "controller.yaml"
	nodePath           = yamlDir + "node.yaml"
	storageClassPath   = yamlDir + "storageclass.yaml"
	testPodPath        = yamlDir + "testpod.yaml"

	// controller statefulset and node daemonset names
	controllerStsName = "spdkcsi-controller"
	nodeDsName        = "spdkcsi-node"
	testPodName       = "spdkcsi-test"
)

func deployConfigs() {
	_, err := framework.RunKubectl("-n", nameSpace, "apply", "-f", configMapPath)
	if err != nil {
		e2elog.Logf("failed to create config map %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "apply", "-f", secretPath)
	if err != nil {
		e2elog.Logf("failed to create secret: %s", err)
	}
}

func deleteConfigs() {
	_, err := framework.RunKubectl("-n", nameSpace, "delete", "-f", configMapPath)
	if err != nil {
		e2elog.Logf("failed to delete config map: %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "delete", "-f", secretPath)
	if err != nil {
		e2elog.Logf("failed to delete secret: %s", err)
	}
}

func deployCsi() {
	_, err := framework.RunKubectl("-n", nameSpace, "apply", "-f", controllerRbacPath)
	if err != nil {
		e2elog.Logf("failed to create controller rbac: %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "apply", "-f", nodeRbacPath)
	if err != nil {
		e2elog.Logf("failed to create node rbac: %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "apply", "-f", controllerPath)
	if err != nil {
		e2elog.Logf("failed to create controller service: %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "apply", "-f", nodePath)
	if err != nil {
		e2elog.Logf("failed to create node service: %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "apply", "-f", storageClassPath)
	if err != nil {
		e2elog.Logf("failed to create storageclass: %s", err)
	}
}

func deleteCsi() {
	_, err := framework.RunKubectl("-n", nameSpace, "delete", "-f", storageClassPath)
	if err != nil {
		e2elog.Logf("failed to delete storageclass: %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "delete", "-f", nodePath)
	if err != nil {
		e2elog.Logf("failed to delete node service: %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "delete", "-f", controllerPath)
	if err != nil {
		e2elog.Logf("failed to delete controller service: %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "delete", "-f", nodeRbacPath)
	if err != nil {
		e2elog.Logf("failed to delete node rbac: %s", err)
	}
	_, err = framework.RunKubectl("-n", nameSpace, "delete", "-f", controllerRbacPath)
	if err != nil {
		e2elog.Logf("failed to delete controller rbac: %s", err)
	}
}

func deployTestPod() {
	_, err := framework.RunKubectl("-n", nameSpace, "apply", "-f", testPodPath)
	if err != nil {
		e2elog.Logf("failed to create test pod: %s", err)
	}
}

func deleteTestPod() {
	_, err := framework.RunKubectl("-n", nameSpace, "delete", "-f", testPodPath)
	if err != nil {
		e2elog.Logf("failed to delete test pod: %s", err)
	}
}

func waitForControllerReady(c kubernetes.Interface, timeout time.Duration) error {
	err := wait.PollImmediate(3*time.Second, timeout, func() (bool, error) {
		sts, err := c.AppsV1().StatefulSets(nameSpace).Get(controllerStsName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if sts.Status.Replicas == sts.Status.ReadyReplicas {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for controller ready: %s", err)
	}
	return nil
}

func waitForNodeServerReady(c kubernetes.Interface, timeout time.Duration) error {
	err := wait.PollImmediate(3*time.Second, timeout, func() (bool, error) {
		ds, err := c.AppsV1().DaemonSets(nameSpace).Get(nodeDsName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for node server ready: %s", err)
	}
	return nil
}

func waitForTestPodReady(c kubernetes.Interface, timeout time.Duration) error {
	err := wait.PollImmediate(3*time.Second, timeout, func() (bool, error) {
		pod, err := c.CoreV1().Pods(nameSpace).Get(testPodName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if string(pod.Status.Phase) == "Running" {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for test pod ready: %s", err)
	}
	return nil
}

var _ = ginkgo.Describe("SPDKCSI", func() {
	f := framework.NewDefaultFramework("spdkcsi")
	ginkgo.BeforeEach(func() {
		deployConfigs()
		deployCsi()
	})

	ginkgo.AfterEach(func() {
		deleteCsi()
		deleteConfigs()
	})

	ginkgo.Context("Test SPDK CSI", func() {
		ginkgo.It("Test SPDK CSI", func() {
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
