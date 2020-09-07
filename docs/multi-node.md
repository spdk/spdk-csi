# Deploy SPDKCSI in multi-node environment

This tutorial is about deploying SPDKCSI on multiple servers. The Kubernetes cluster contains three nodes, one master and two workers. And there is one dedicated storage node with NVMe device and runs SPDK software stack. Kubernetes worker nodes access SPDK storage through NVMe over TCP.

## Nodes

| Hostname   | IP                | Purpose                           | OS           |
| ---------- | ----------------- | --------------------------------- | ------------ |
| k8s-master | 192.168.12.200/24 | Kubernetes cluster master node    | Ubuntu-18.04 |
| k8s-node1  | 192.168.12.201/24 | Kubernetes cluster worker node #1 | Ubuntu-18.04 |
| k8s-node2  | 192.168.12.202/24 | Kubernetes cluster worker node #2 | Ubuntu-18.04 |
| spdk-node  | 192.168.12.203/24 | Storage node running SPDK service | Ubuntu-18.04 |

## Deploy Kubernetes cluster

Deploy Kubernetes cluster (one master, two workers) on hosts `k8s-master`, `k8s-node1` and `k8s-node2`, using [kubeadm](https://github.com/kubernetes/kubeadm), [kubespray](https://github.com/kubernetes-sigs/kubespray) or other deployment tools.

## Build SPDKCSI image

SPDKCSI image `spdkcsi/spdkcsi:canary` needs to be manually built on all Kubernetes worker nodes before deployment.
Run the one-liner below on hosts `k8s-node1` and `k8s-node2` where CSI controller and node servers will be scheduled.
```bash
k8s-node1:~$ docker run -it --rm -v /var/run/docker.sock:/var/run/docker.sock golang:1.14 bash -c "apt update && apt install -y make git docker.io && git clone https://review.spdk.io/gerrit/spdk/spdk-csi && cd spdk-csi && make image"
```

## Start SPDK service

### Build SPDK
Please follow [SPDK Getting Started](https://spdk.io/doc/getting_started.html) to build and setup SPDK on host `spdk-node`.

### Start SPDK
```bash
# Assume your cloned SPDK source code to ~/spdk.
# Start SPDK target
spdk-node:~/spdk$ sudo build/bin/spdk_tgt
# Create NVMe bdev
spdk-node:~/spdk$ sudo scripts/rpc.py bdev_nvme_attach_controller -b NVMe0 -t PCIe -a <nvme-device-pcie-addr>
# Create logical volume store based on NVMe bdev
spdk-node:~/spdk$ sudo scripts/rpc.py bdev_lvol_create_lvstore NVMe0n1 lvs
```
Please reference SPDK [Block Device User Guide](https://spdk.io/doc/bdev.html) and [Logical Volumes](https://spdk.io/doc/logical_volumes.html) document.

### Start JSON RPC http proxy
JSON RPC http proxy enables remote access to SPDK service.
Below commands start proxy on port 9009 with specified username and password.
```bash
# Accept remote JSON RPC requests on 192.168.12.203:9009 with token
# username: spdkcsiuser, password: spdkcsipass
spdk-node:~/spdk$ sudo scripts/rpc_http_proxy.py 192.168.12.203 9009 spdkcsiuser spdkcsipass
```

## Deploy SPDKCSI

Run SPDKCSI deployment scripts on host `k8s-master`.

- Clone code
  ```bash
  k8s-master:~$ git clone https://review.spdk.io/gerrit/spdk/spdk-csi
  k8s-master:~$ cd spdk-csi/deploy/kubernetes
  ```

- Update `config-map.yaml` config.json section
  ```yaml
  config.json: |-
    {
      "nodes": [
        {
          "name": "spdk-node",
          "rpcURL": "http://192.168.12.203:9009",
          "targetType": "nvme-tcp",
          "targetAddr": "192.168.12.203"
        }
      ]
    }
  ```

- Update `secret.yaml` secret.json section
  ```yaml
  secret.json: |-
    {
      "rpcTokens": [
        {
          "name": "spdk-node",
          "username": "spdkcsiuser",
          "password": "spdkcsipass"
        }
      ]
    }
  ```

- Deploy SPDKCSI drivers
  ```bash
  k8s-master:~/spdk-csi/deploy/kubernetes$ ./deploy.sh

  # Check CSI controller and node drivers readiness
  k8s-master:~/spdk-csi/deploy/kubernetes$ kubectl get pods
  NAME                   READY   STATUS    RESTARTS   AGE
  spdkcsi-controller-0   3/3     Running   0          10s
  spdkcsi-node-5rf97     2/2     Running   0          10s
  spdkcsi-node-ptxk8     2/2     Running   0          10s
  ```

- Deploy test pod
Test pod applies 256MB block storage from SPDKCSI driver and mount to /spdkvol directory. The storage is provided by SPDK node and imported to Kubernetes worker node through NVMe-TCP.
  ```bash
  k8s-master:~/spdk-csi/deploy/kubernetes$ kubectl apply -f testpod.yaml

  # Check test pod readiness
  k8s-master:~/spdk-csi/deploy/kubernetes$ kubectl get pods/spdkcsi-test
  NAME           READY   STATUS    RESTARTS   AGE
  spdkcsi-test   1/1     Running   0          47s

  # Check mounted volume in test pod
  k8s-master:~/spdk-csi/deploy/kubernetes$ kubectl exec -it spdkcsi-test mount | grep spdk
  /dev/disk/by-id/nvme-6533e2c6-d69b-4529-8666-504cb1ee63a9_spdkcsi-sn on /spdkvol type ext4 (rw,relatime)
  ```

- Delete test pod and CSI drivers
  ```bash
  k8s-master:~/spdk-csi/deploy/kubernetes$ kubectl delete -f testpod.yaml
  k8s-master:~/spdk-csi/deploy/kubernetes$ ./deploy.sh teardown
  ```

## Debug

- Check SPDKCSI driver logs
  ```bash
  k8s-master:~$ kubectl logs spdkcsi-controller-0 spdkcsi-controller
  k8s-master:~$ kubectl logs spdkcsi-node-5rf97 spdkcsi-node
  ```

- Check NVMe event logs
Find the worker node where test pod is scheduled with `kubectl get pods/spdkcsi-test -o wide`
Login according worker node and check kernel logs.
  ```bash
  k8s-node1:~$ dmesg | grep -i nvm
  [ 1150.536656] nvme nvme0: new ctrl: NQN "nqn.2020-04.io.spdk.csi:uuid:57d00891-f065-4fce-9f04-6ee4ab59ea42", addr 192.168.122.203:4420
  [ 1150.537187] nvme0n1: detected capacity change from 0 to 268435456
  [ 1151.587243] EXT4-fs (nvme0n1): mounted filesystem with ordered data mode. Opts: (null)
  [ 1683.691621] nvme nvme0: Removing ctrl: NQN "nqn.2020-04.io.spdk.csi:uuid:57d00891-f065-4fce-9f04-6ee4ab59ea42"
  ```
