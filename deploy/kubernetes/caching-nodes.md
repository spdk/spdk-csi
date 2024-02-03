### Caching nodes volume provisioning

We now also have the concept of caching-nodes. These will be Kubernetes nodes, which reside within the k8s compute cluster as normal worker nodes and can have any PODs deployed. However, they have an NVMe disk locally attached. If there are multiple NVMe disks attached, it will use the first one.

Caching nodes are a special kind of node that provides storage by mounting a local NVMe disk. For a node to qualify as caching node, it should have the following properties. (A seperate usermanual will be provided to prepare worker nodes)
<TODO>

On SimplyBlock the general workflow to use a Caching node is:
1. Create a logical volume. `sbcli lvol add lvolx1 25G pool01`
2. List the caching nodes using: `sbcli caching-node list`
3. Connect caching node with lvol using: `sbcli caching-node connect <node-uuid> <lvol-uuid>`

All the nodes that qualify as caching nodes that are tainted with "type: cache" have all the caching containers.

