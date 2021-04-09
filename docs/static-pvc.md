# Static PVC with SPDK-CSI

This document outlines how to create static PV and static PVC from existing SPDK logical volume.

## Create static PV

### NVMe over Fabrics Target

Refer to the following [SPDK documents](https://spdk.io/doc/nvmf.html) to create and publish SPDK logical volume manually.

To create the static PV you need to know the `model`, `nqn`, `lvol`, `targetAddr`,
`targetPort`, and `targetType` name of the SPDK logical volume.

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pv.kubernetes.io/provisioned-by: csi.spdk.io
  finalizers:
  - kubernetes.io/pv-protection
  name: pv-static
spec:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 256Mi
  csi:
    driver: csi.spdk.io
    fsType: ext4
    volumeAttributes:
      # MODEL_NUMBER, set by the `nvmf_create_subsystem` method
      model: aa481c21-26f8-4056-87fa-cd306f69a71e
      # Subsystem NQN (ASCII), set by the `nvmf_create_subsystem` method
      nqn: nqn.2020-04.io.spdk.csi:uuid:aa481c21-26f8-4056-87fa-cd306f69a71e
      # The listen address to an NVMe-oF subsystemset, set by the `nvmf_subsystem_add_listener` method
      targetAddr: 127.0.0.1
      targetPort: "4420"
      # transport type, TCP or RDMA
      targetType: TCP
    # volumeHandle should be same as lvol store name(uuid)
    volumeHandle: aa481c21-26f8-4056-87fa-cd306f69a71e
  persistentVolumeReclaimPolicy: Retain
  storageClassName: spdkcsi-sc
  volumeMode: Filesystem
```

```bash
$ kubectl create -f pv-static.yaml
persistentvolume/pv-static created
```

### iSCSI Target

Refer to the following [SPDK documents](https://spdk.io/doc/iscsi.html) to create and publish SPDK logical volume manually.

To create the static PV you need to know the `lvol`, `targetAddr`and `targetPort` of the SPDK logical volume.

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pv.kubernetes.io/provisioned-by: csi.spdk.io
  name: pv-static
spec:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 256Mi
  csi:
    driver: csi.spdk.io
    fsType: ext4
    volumeAttributes:
      # number Initiator group tag, the default value is `iqn.2016-06.io.spdk:`+ `volumeHandle`
      iqn: iqn.2016-06.io.spdk:c0cd9559-cd6e-43b6-98af-45196e41655f
      # iSCSI transport address, set by the `iscsi_create_portal_group` method
      targetAddr: 127.0.0.1
      targetPort: "3260"
      targetType: iscsi
    # volumeHandle should be same as lvol store name(uuid)
    volumeHandle: c0cd9559-cd6e-43b6-98af-45196e41655f
  persistentVolumeReclaimPolicy: Retain
  storageClassName: spdkcsi-sc
  volumeMode: Filesystem
```

```bash
$ kubectl create -f pv-static.yaml
persistentvolume/pv-static created
```

**Note:** SPDK-CSI does not supports logical volume deletion for static PV.
`persistentVolumeReclaimPolicy` in PV spec must be set to `Retain` to avoid PV delete attempt in csi-provisioner.

## Create static PVC

To create the static PVC you need to know the PV name which is created above.

```yaml
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: pvc-static
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 256Mi
  # As a functional test, volumeName is same as PV name
  volumeName: pv-static
  storageClassName: spdkcsi-sc
```

```bash
$ kubectl create -f pvc-static.yaml
persistentvolumeclaim/pvc-static created
```

## Create test Pod

```yaml
kind: Pod
apiVersion: v1
metadata:
  name: spdkcsi-test
spec:
  containers:
  - name: alpine
    image: alpine:3
    imagePullPolicy: "IfNotPresent"
    command: ["sleep", "365d"]
    volumeMounts:
    - mountPath: "/spdkvol"
      name: spdk-volume
  volumes:
  - name: spdk-volume
    persistentVolumeClaim:
      claimName: pvc-static
```

```bash
$ kubectl create -f spdkcsi-test.yaml
pod/spdkcsi-test created
```
