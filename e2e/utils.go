package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega" //nolint
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

const (
	nameSpace = "default"

	// deployment yaml files
	yamlDir                  = "../deploy/kubernetes/"
	driverPath               = yamlDir + "driver.yaml"
	secretPath               = yamlDir + "secret.yaml"
	controllerRbacPath       = yamlDir + "controller-rbac.yaml"
	nodeRbacPath             = yamlDir + "node-rbac.yaml"
	controllerPath           = yamlDir + "controller.yaml"
	nodePath                 = yamlDir + "node.yaml"
	storageClassPath         = yamlDir + "storageclass.yaml"
	pvcPath                  = "pvc.yaml"
	testPodPath              = "testpod.yaml"
	smaNvmfConfigPath        = "sma-nvmf.yaml"
	smaNvmeConfigPath        = "sma-nvme.yaml"
	smaVirtioBlkConfigPath   = "sma-virtioblk.yaml"
	opiNvmeConfigPath        = "opi-nvme.yaml"
	opiVirtioBlkConfigPath   = "opi-virtioblk.yaml"
	multiPvcsPath            = "multi-pvc.yaml"
	testPodWithMultiPvcsPath = "testpod-multi-pvc.yaml"

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

func deploySmaNvmfConfig() {
	_, err := framework.RunKubectl(nameSpace, "apply", "-f", smaNvmfConfigPath)
	if err != nil {
		e2elog.Logf("failed to create Sma Nvmf configmap %s", err)
	}
}

func deleteSmaNvmfConfig() {
	_, err := framework.RunKubectl(nameSpace, "delete", "-f", smaNvmfConfigPath)
	if err != nil {
		e2elog.Logf("failed to delete Sma Nvmf configmap %s", err)
	}
}

func deploySmaNvmeConfig() {
	_, err := framework.RunKubectl(nameSpace, "apply", "-f", smaNvmeConfigPath)
	if err != nil {
		e2elog.Logf("failed to create Sma Nvme configmap %s", err)
	}
}

func deleteSmaNvmeConfig() {
	_, err := framework.RunKubectl(nameSpace, "delete", "-f", smaNvmeConfigPath)
	if err != nil {
		e2elog.Logf("failed to delete Sma Nvme configmap %s", err)
	}
}

func deploySmaVirtioBlkConfig() {
	_, err := framework.RunKubectl(nameSpace, "apply", "-f", smaVirtioBlkConfigPath)
	if err != nil {
		e2elog.Logf("failed to create Sma VirtioBlk configmap %s", err)
	}
}

func deleteSmaVirtioBlkConfig() {
	_, err := framework.RunKubectl(nameSpace, "delete", "-f", smaVirtioBlkConfigPath)
	if err != nil {
		e2elog.Logf("failed to delete Sma VirtioBlk configmap %s", err)
	}
}

func deployOpiNvmeConfig() {
	_, err := framework.RunKubectl(nameSpace, "apply", "-f", opiNvmeConfigPath)
	if err != nil {
		e2elog.Logf("failed to create Opi Nvme configmap %s", err)
	}
}

func deleteOpiNvmeConfig() {
	_, err := framework.RunKubectl(nameSpace, "delete", "-f", opiNvmeConfigPath)
	if err != nil {
		e2elog.Logf("failed to delete Opi Nvme configmap %s", err)
	}
}

func deployOpiVirtioBlkConfig() {
	_, err := framework.RunKubectl(nameSpace, "apply", "-f", opiVirtioBlkConfigPath)
	if err != nil {
		e2elog.Logf("failed to create Opi VirtioBlk configmap %s", err)
	}
}

func deleteOpiVirtioBlkConfig() {
	_, err := framework.RunKubectl(nameSpace, "delete", "-f", opiVirtioBlkConfigPath)
	if err != nil {
		e2elog.Logf("failed to delete Opi VirtioBlk configmap %s", err)
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

func deleteTestPodForce() {
	_, err := framework.RunKubectl(nameSpace, "delete", "--force", "-f", testPodPath)
	if err != nil {
		e2elog.Logf("failed to delete test pod: %s", err)
	}
}

func deleteTestPodWithTimeout(timeout time.Duration) error {
	_, err := framework.NewKubectlCommand(nameSpace, "delete", "-f", testPodPath).
		WithTimeout(time.After(timeout)).Exec()
	return err
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

func deployTestPodWithMultiPvcs() {
	_, err := framework.RunKubectl(nameSpace, "apply", "-f", testPodWithMultiPvcsPath)
	if err != nil {
		e2elog.Logf("failed to create test pod with multiple pvcs: %s", err)
	}
}

func deleteTestPodWithMultiPvcs() {
	_, err := framework.RunKubectl(nameSpace, "delete", "-f", testPodWithMultiPvcsPath)
	if err != nil {
		e2elog.Logf("failed to delete test pod with multiple pvcs: %s", err)
	}
}

func deployMultiPvcs() {
	_, err := framework.RunKubectl(nameSpace, "apply", "-f", multiPvcsPath)
	if err != nil {
		e2elog.Logf("failed to create pvcs: %s", err)
	}
}

func deleteMultiPvcs() {
	_, err := framework.RunKubectl(nameSpace, "delete", "-f", multiPvcsPath)
	if err != nil {
		e2elog.Logf("failed to delete pvcs: %s", err)
	}
}

func deleteMultiPvcsAndTestPodWithMultiPvcs() {
	deleteTestPodWithMultiPvcs()
	deleteMultiPvcs()
}

// rolloutNodeServer Use the delete corresponding pod to simulate a rollout. In this way, when the function returns,
// the state of the NodeServer has definitely changed, which is convenient for subsequent state detection.
func rolloutNodeServer() {
	_, err := framework.RunKubectl(nameSpace, "delete", "pod", "-l", fmt.Sprintf("app=%s", nodeDsName))
	if err != nil {
		e2elog.Logf("failed to rollout node server: %s", err)
	}
}

func rolloutControllerServer() {
	_, err := framework.RunKubectl(nameSpace, "delete", "pod", "-l", fmt.Sprintf("app=%s", controllerStsName))
	if err != nil {
		e2elog.Logf("failed to rollout controller server: %s", err)
	}
}

func waitForControllerReady(c kubernetes.Interface, timeout time.Duration) error { //nolint:unparam //Keep timeout parameter, it may be used in the future
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

func verifyNodeServerLog(expLogList []string) error {
	log, err := framework.RunKubectl(nameSpace, "logs", "-l", "app=spdkcsi-node", "-c", "spdkcsi-node", "--tail", "-1")
	if err != nil {
		return fmt.Errorf("failed to obtain the log from node server: %w", err)
	}

	for _, expLog := range expLogList {
		if !strings.Contains(log, expLog) {
			return fmt.Errorf("failed to catch the log about %s", expLog)
		}
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

func waitForTestPodGone(c kubernetes.Interface) error {
	err := wait.PollImmediate(3*time.Second, 5*time.Minute, func() (bool, error) {
		_, err := c.CoreV1().Pods(nameSpace).Get(ctx, testPodName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for test pod gone: %w", err)
	}
	return nil
}

func waitForPvcGone(c kubernetes.Interface, pvcName string) error {
	err := wait.PollImmediate(3*time.Second, 5*time.Minute, func() (bool, error) {
		_, err := c.CoreV1().PersistentVolumeClaims(nameSpace).Get(ctx, pvcName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for pvc (%s) gone: %w", pvcName, err)
	}
	return nil
}

//nolint:unparam // Currently, "ns" always receives "nameSpace", skip this linter checking
func execCommandInPod(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (stdOut, stdErr string) {
	podPot := getCommandInPodOpts(f, c, ns, opt)
	stdOut, stdErr, err := f.ExecWithOptions(podPot)
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}
	Expect(err).ShouldNot(HaveOccurred())
	return stdOut, stdErr
}

func getCommandInPodOpts(f *framework.Framework, c, ns string, opt *metav1.ListOptions) framework.ExecOptions {
	cmd := []string{"/bin/sh", "-c", c}
	podList, err := f.PodClientNS(ns).List(ctx, *opt)
	framework.ExpectNoError(err)
	Expect(podList.Items).NotTo(BeNil())
	Expect(err).ShouldNot(HaveOccurred())

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
	err := waitForTestPodGone(f.ClientSet)
	if err != nil {
		return err
	}

	deployTestPod()
	err = waitForTestPodReady(f.ClientSet, 5*time.Minute)
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

func checkDataPersistForMultiPvcs(f *framework.Framework) error {
	dataContents := []string{
		"Data that needs to be stored to vol1",
		"Data that needs to be stored to vol2",
		"Data that needs to be stored to vol3",
	}
	// write data to PVC
	dataPaths := []string{
		"/spdkvol1/test",
		"/spdkvol2/test",
		"/spdkvol3/test",
	}
	opt := metav1.ListOptions{
		LabelSelector: "app=spdkcsi-pvc",
	}
	for i := 0; i < len(dataPaths); i++ {
		execCommandInPod(f, fmt.Sprintf("echo %s > %s", dataContents[i], dataPaths[i]), nameSpace, &opt)
	}

	deleteTestPodWithMultiPvcs()
	err := waitForTestPodGone(f.ClientSet)
	if err != nil {
		return err
	}

	deployTestPodWithMultiPvcs()
	err = waitForTestPodReady(f.ClientSet, 3*time.Minute)
	if err != nil {
		return err
	}

	// read data from PVC
	for i := 0; i < len(dataPaths); i++ {
		persistData, stdErr := execCommandInPod(f, fmt.Sprintf("cat %s", dataPaths[i]), nameSpace, &opt)
		Expect(stdErr).Should(BeEmpty())
		if !strings.Contains(persistData, dataContents[i]) {
			return fmt.Errorf("data not persistent: expected data %s received data %s ", dataContents[i], persistData)
		}
	}
	return err
}
