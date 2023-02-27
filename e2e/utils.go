package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega" //nolint
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

const (
	nameSpace = "default"

	// deployment yaml files
	yamlDir            = "../deploy/kubernetes/"
	driverPath         = yamlDir + "driver.yaml"
	secretPath         = yamlDir + "secret.yaml"
	controllerRbacPath = yamlDir + "controller-rbac.yaml"
	nodeRbacPath       = yamlDir + "node-rbac.yaml"
	controllerPath     = yamlDir + "controller.yaml"
	nodePath           = yamlDir + "node.yaml"
	storageClassPath   = yamlDir + "storageclass.yaml"
	pvcPath            = "pvc.yaml"
	testPodPath        = "testpod.yaml"

	// controller statefulset and node daemonset names
	controllerStsName = "spdkcsi-controller"
	nodeDsName        = "spdkcsi-node"
	testPodName       = "spdkcsi-test"
)

var ctx = context.TODO()

func deployConfigs(configMapData string) {
	configMapData = "--from-literal=config.json=" + configMapData
	_, err := framework.RunKubectl(nameSpace, "create", "configmap", "spdkcsi-cm", configMapData)
	if err != nil {
		e2elog.Logf("failed to create config map %s", err)
	}
	_, err = framework.RunKubectl(nameSpace, "apply", "-f", secretPath)
	if err != nil {
		e2elog.Logf("failed to create secret: %s", err)
	}
}

func deleteConfigs() {
	_, err := framework.RunKubectl(nameSpace, "delete", "configmap", "spdkcsi-cm")
	if err != nil {
		e2elog.Logf("failed to delete config map: %s", err)
	}
	_, err = framework.RunKubectl(nameSpace, "delete", "-f", secretPath)
	if err != nil {
		e2elog.Logf("failed to delete secret: %s", err)
	}
}

var csiYamls = []string{
	driverPath,
	controllerRbacPath,
	nodeRbacPath,
	controllerPath,
	nodePath,
	storageClassPath,
}

func deployCsi() {
	for _, yamlName := range csiYamls {
		_, err := framework.RunKubectl(nameSpace, "apply", "-f", yamlName)
		if err != nil {
			e2elog.Logf("failed to create %s: %s", yamlName, err)
		}
	}
}

func deleteCsi() {
	cnt := len(csiYamls)
	// delete objects in reverse order
	for i := range csiYamls {
		yamlName := csiYamls[cnt-1-i]
		_, err := framework.RunKubectl(nameSpace, "delete", "-f", yamlName)
		if err != nil {
			e2elog.Logf("failed to delete %s: %s", yamlName, err)
		}
	}
}

func deployTestPod() {
	_, err := framework.RunKubectl(nameSpace, "apply", "-f", testPodPath)
	if err != nil {
		e2elog.Logf("failed to create test pod: %s", err)
	}
}

func deleteTestPod() {
	_, err := framework.RunKubectl(nameSpace, "delete", "-f", testPodPath)
	if err != nil {
		e2elog.Logf("failed to delete test pod: %s", err)
	}
}

func deployPVC() {
	_, err := framework.RunKubectl(nameSpace, "apply", "-f", pvcPath)
	if err != nil {
		e2elog.Logf("failed to create pvc: %s", err)
	}
}

func deletePVC() {
	_, err := framework.RunKubectl(nameSpace, "delete", "-f", pvcPath)
	if err != nil {
		e2elog.Logf("failed to delete pvc: %s", err)
	}
}

func deletePVCAndTestPod() {
	deleteTestPod()
	deletePVC()
}

func waitForControllerReady(c kubernetes.Interface, timeout time.Duration) error {
	err := wait.PollImmediate(3*time.Second, timeout, func() (bool, error) {
		sts, err := c.AppsV1().StatefulSets(nameSpace).Get(ctx, controllerStsName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if sts.Status.Replicas == sts.Status.ReadyReplicas {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for controller ready: %w", err)
	}
	return nil
}

func waitForNodeServerReady(c kubernetes.Interface, timeout time.Duration) error {
	err := wait.PollImmediate(3*time.Second, timeout, func() (bool, error) {
		ds, err := c.AppsV1().DaemonSets(nameSpace).Get(ctx, nodeDsName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for node server ready: %w", err)
	}
	return nil
}

func waitForTestPodReady(c kubernetes.Interface, timeout time.Duration) error {
	err := wait.PollImmediate(3*time.Second, timeout, func() (bool, error) {
		pod, err := c.CoreV1().Pods(nameSpace).Get(ctx, testPodName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if string(pod.Status.Phase) == "Running" {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for test pod ready: %w", err)
	}
	return nil
}

func execCommandInPod(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (stdOut, stdErr string) {
	podPot := getCommandInPodOpts(f, c, ns, opt)
	stdOut, stdErr, err := f.ExecWithOptions(podPot)
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}
	Expect(err).Should(BeNil())
	return stdOut, stdErr
}

func getCommandInPodOpts(f *framework.Framework, c, ns string, opt *metav1.ListOptions) framework.ExecOptions {
	cmd := []string{"/bin/sh", "-c", c}
	podList, err := f.PodClientNS(ns).List(ctx, *opt)
	framework.ExpectNoError(err)
	Expect(podList.Items).NotTo(BeNil())
	Expect(err).Should(BeNil())

	return framework.ExecOptions{
		Command:            cmd,
		PodName:            podList.Items[0].Name,
		Namespace:          ns,
		ContainerName:      podList.Items[0].Spec.Containers[0].Name,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: true,
	}
}

func checkDataPersist(f *framework.Framework) error {
	data := "Data that needs to be stored"
	// write data to PVC
	dataPath := "/spdkvol/test"
	opt := metav1.ListOptions{
		LabelSelector: "app=spdkcsi-pvc",
	}
	execCommandInPod(f, fmt.Sprintf("echo %s > %s", data, dataPath), nameSpace, &opt)

	deleteTestPod()
	deployTestPod()
	err := waitForTestPodReady(f.ClientSet, 5*time.Minute)
	if err != nil {
		return err
	}

	// read data from PVC
	persistData, stdErr := execCommandInPod(f, fmt.Sprintf("cat %s", dataPath), nameSpace, &opt)
	Expect(stdErr).Should(BeEmpty())
	if !strings.Contains(persistData, data) {
		return fmt.Errorf("data not persistent: expected data %s received data %s ", data, persistData)
	}

	return err
}
