#!/bin/bash

# list in creation order
files=(driver config-map nodeserver-config-map secret controller-rbac node-rbac controller node storageclass)

if [ "$1" = "teardown" ]; then
	# delete in reverse order
	for ((i = ${#files[@]} - 1; i >= 0; i--)); do
		echo "=== kubectl delete -f ${files[i]}.yaml"
		kubectl delete -f "${files[i]}.yaml"
	done
else
	for ((i = 0; i <= ${#files[@]} - 1; i++)); do
		echo "=== kubectl apply -f ${files[i]}.yaml"
		kubectl apply -f "${files[i]}.yaml"
	done
fi

# echo "Deploying Caching node..."

# echo "adding all the k8s nodes tainted with simplyblock:cache as caching nodes"
# kubectl apply -f caching-nodes.yaml

# kubectl wait --for=condition=ready pod -l app=caching-node

# service="caching-node-service"
# node_port=$(kubectl get services $service -o=jsonpath='{.spec.ports[0].nodePort}' 2>/dev/null)

# echo "$service is running on port $node_port"

# curl --location 'http://3.140.238.66/cachingnode/' \
# 	--header 'Content-Type: application/json' \
# 	--header 'Authorization: 1a215589-e08f-47aa-bafb-3ee68d719e35 OdXPGPY2ITUrdAK6xVcv' \
# 	--data '{
#     "cluster_id": "1a215589-e08f-47aa-bafb-3ee68d719e35",
#     "node_ip": "10.0.4.13:31484",
#     "iface_name": "eth0"
# }
# '

# ## preparing nodes
# sudo echo "vm.nr_hugepages=2048" >> /etc/sysctl.conf
# sudo sysctl -p

# mount -t hugetlbfs -o size=2g nodev /mnt/huge

# logic on the node side
# 1. get the caching node UUID
# 2. deploy the caching node
