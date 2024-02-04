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

CLUSTER_ID='0afa8f0c-03c2-4223-800c-a4d5b39f0010'
MGMT_IP='3.144.225.78'
CLUSTER_SECRET=vbpuEOtrs12s85JtBRQH

echo "Deploying Caching node..."
kubectl apply -f caching-node.yaml
kubectl wait --for=condition=ready pod -l app=caching-node

for node in $(kubectl get pods -l app=caching-node -owide | awk 'NR>1 {print $6}'); do
	echo "adding caching node: $node"

	curl --location "http://${MGMT_IP}/cachingnode/" \
		--header "Content-Type: application/json" \
		--header "Authorization: ${CLUSTER_ID} ${CLUSTER_SECRET}" \
		--data '{
		"cluster_id": "'"${CLUSTER_ID}"'",
		"node_ip": "'"${node}:5000"'",
		"iface_name": "eth0"
	}
	'
done
