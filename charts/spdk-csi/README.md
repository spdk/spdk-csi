# Installation with Helm 3

Follow this guide to install the SPDK-CSI Driver for Kubernetes.

## Prerequisites

### [Install Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

### Build image

 ```console
 $ make image
 $ cd deploy/spdk
 $ sudo docker build -t spdkdev .
 ```
 **_NOTE:_**
Kubernetes nodes must pre-allocate hugepages in order for the node to report its hugepage capacity. A node can pre-allocate huge pages for multiple sizes.

## Install latest CSI Driver via `helm install`

```console
$ cd charts
$ helm install spdk-csi ./spdk-csi --namespace spdk-csi
```

## After installation succeeds, you can get a status of Chart

```console
$ helm status "spdk-csi"
```

## Delete Chart

If you want to delete your Chart, use this command

```bash
helm uninstall "spdk-csi" --namespace "spdk-csi"
```

If you want to delete the namespace, use this command

```bash
kubectl delete namespace spdk-csi
```
