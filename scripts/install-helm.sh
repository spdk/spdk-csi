#!/bin/bash -e

TEMP="/tmp/spdkcsi-helm-test"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

HELM="helm"
HELM_VERSION="v3.3.0"
arch="${ARCH:-}"
SPDKCSI_CHART_NAME="spdk-csi"
DEPLOY_TIMEOUT=300

detectArch() {
	case "$(uname -m)" in
	"x86_64" | "amd64")
		arch="amd64"
		;;
	"aarch64")
		arch="arm64"
		;;
	*)
		echo "Couldn't translate 'uname -m' output to an available arch."
		echo "Try setting ARCH environment variable to your system arch:"
		echo "amd64, x86_64, aarch64"
		exit 1
		;;
	esac
}

install() {
	if [ "$hasHelm" = false ]; then
		mkdir -p ${TEMP}
		wget "https://get.helm.sh/helm-${HELM_VERSION}-${dist}-${arch}.tar.gz" -O "${TEMP}/helm.tar.gz"
		tar -C "${TEMP}" -zxvf "${TEMP}/helm.tar.gz"
	fi
	echo "Helm install successful"
}

install_spdkcsi_helm_charts() {
	NAMESPACE=$1
	if [ -z "$NAMESPACE" ]; then
		NAMESPACE="default"
	fi
	# install spdk-csi-spdkfs and spdk-csi-rbd charts
	"${HELM}" install --debug --namespace ${NAMESPACE} ${SPDKCSI_CHART_NAME} "${SCRIPT_DIR}"/../charts/spdk-csi

	check_deployment_status spdkdev ${NAMESPACE}
	check_daemonset_status spdkcsi-node ${NAMESPACE}
	check_statefulset_status spdkcsi-controller ${NAMESPACE}
}

function check_deployment_status() {
	LABEL=$1
	NAMESPACE=$2
	echo "Checking Deployment status for label $LABEL in Namespace $NAMESPACE"
	for ((retry = 0; retry <= DEPLOY_TIMEOUT; retry = retry + 10)); do
		total_replicas=$(kubectl get deployment "$LABEL" -n "$NAMESPACE" -o jsonpath='{.status.replicas}')

		ready_replicas=$(kubectl get deployment "$LABEL" -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}')
		if [ "$total_replicas" != "$ready_replicas" ]; then
			echo "Total replicas $total_replicas is not equal to ready count $ready_replicas"
			kubectl get deployment -l "$LABEL" -n "$NAMESPACE"
			sleep 10
		else
			echo "Total replicas $total_replicas is equal to ready count $ready_replicas"
			break
		fi
	done

	if [ "$retry" -gt "$DEPLOY_TIMEOUT" ]; then
		exit_with_details "[Timeout] Failed to get deployment" "${NAMESPACE}"
	fi
}

function check_statefulset_status() {
	LABEL=$1
	NAMESPACE=$2
	echo "Checking StatefulSet status for label $LABEL in Namespace $NAMESPACE"
	for ((retry = 0; retry <= DEPLOY_TIMEOUT; retry = retry + 10)); do
		total_replicas=$(kubectl get statefulset "$LABEL" -n "$NAMESPACE" -o jsonpath='{.status.replicas}')

		ready_replicas=$(kubectl get statefulset "$LABEL" -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}')
		if [ "$total_replicas" != "$ready_replicas" ]; then
			echo "Total replicas $total_replicas is not equal to ready count $ready_replicas"
			kubectl get statefulset "$LABEL" -n "$NAMESPACE"
			sleep 10
		else
			echo "Total replicas $total_replicas is equal to ready count $ready_replicas"
			break
		fi
	done

	if [ "$retry" -gt "$DEPLOY_TIMEOUT" ]; then
		exit_with_details "[Timeout] Failed to get statefulset" "${NAMESPACE}"
	fi
}

function check_daemonset_status() {
	LABEL=$1
	NAMESPACE=$2
	echo "Checking Daemonset status for label $LABEL in Namespace $NAMESPACE"
	for ((retry = 0; retry <= DEPLOY_TIMEOUT; retry = retry + 10)); do
		total_replicas=$(kubectl get daemonset "$LABEL" -n "$NAMESPACE" -o jsonpath='{.status.numberAvailable}')

		ready_replicas=$(kubectl get daemonset "$LABEL" -n "$NAMESPACE" -o jsonpath='{.status.numberReady}')
		if [ "$total_replicas" != "$ready_replicas" ]; then
			echo "Total replicas $total_replicas is not equal to ready count $ready_replicas"
			kubectl get daemonset "$LABEL" -n "$NAMESPACE"
			sleep 10
		else
			echo "Total replicas $total_replicas is equal to ready count $ready_replicas"
			break

		fi
	done

	if [ "$retry" -gt "$DEPLOY_TIMEOUT" ]; then
		exit_with_details "[Timeout] Failed to get daemonset" "${NAMESPACE}"
	fi
}

cleanup_spdkcsi_helm_charts() {
	NAMESPACE=$1
	if [ -z "$NAMESPACE" ]; then
		NAMESPACE="default"
	fi
	"${HELM}" uninstall --debug ${SPDKCSI_CHART_NAME} --namespace ${NAMESPACE}
}

helm_reset() {
	# shellcheck disable=SC2021
	rm -rf "${TEMP}"
}

function exit_with_details() {
	local ERRORMSG=$1
	local NAMESPACE=$2
	echo "$ERRORMSG"
	if [ -n "$NAMESPACE" ]; then
		echo "=========== pods details in $NAMESPACE ============"
		kubectl get pods -n "$NAMESPACE" -o yaml

		echo "=========== pods logs in $NAMESPACE ==============="
		for p in $(kubectl get pods -o name | grep spdk); do
			echo "pod: $p"
			kubectl logs "$p" --all-containers --tail 200
			echo "===================================================="
		done
	fi
	exit 1
}

if [ -z "${arch}" ]; then
	detectArch
fi

if ! helm_loc="$(type -p "helm")" || [[ -z ${helm_loc} ]]; then
	dist="$(uname -s)"
	# shellcheck disable=SC2021
	dist=$(echo "${dist}" | tr "[A-Z]" "[a-z]")
	HELM="${TEMP}/${dist}-${arch}/helm"
	hasHelm=false
fi

case "${1:-}" in
up)
	install
	;;
clean)
	helm_reset
	;;
install-spdkcsi)
	install_spdkcsi_helm_charts "$2"
	;;
cleanup-spdkcsi)
	cleanup_spdkcsi_helm_charts "$2"
	;;
*)
	echo "usage:" >&2
	echo "  $0 up" >&2
	echo "  $0 clean" >&2
	echo "  $0 install-spdkcsi" >&2
	echo "  $0 cleanup-spdkcsi" >&2
	;;
esac
