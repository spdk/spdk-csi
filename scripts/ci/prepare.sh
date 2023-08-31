#!/bin/bash -e

DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# shellcheck source=scripts/ci/env
source "${DIR}/env"
# shellcheck source=scripts/ci/common.sh
source "${DIR}/common.sh"

trap cleanup ERR

PROMPT_FLAG=${PROMPT_FLAG:-true}

if [[ $(id -u) != "0" ]]; then
	echo "Go away user, come back as root."
	exit 1
fi

# Prepare VM for running xPU tests on amd64 hosts.
# To avoid this invoke the script with -x (no vm)
if [ "${ARCH}" = amd64 ]; then
	PREPARE_VM=yes
else
	PREPARE_VM=no
fi

while getopts 'yu:p:x' optchar; do
	case "$optchar" in
		y)
			PROMPT_FLAG=false
			;;
		u)
			DOCKERHUB_USER="$OPTARG"
			;;
		p)
			DOCKERHUB_SECRET="$OPTARG"
			;;
		x)
			PREPARE_VM=no
			;;
		*)
			echo "$0: invalid argument '$optchar'"
			exit 1
			;;
	esac
done

if $PROMPT_FLAG; then
	echo "This script is meant to run on CI nodes."
	echo "It will install packages and docker images on current host."
	echo "Make sure you understand what it does before going on."
	read -r -p "Do you want to continue (yes/no)? " yn
	case "${yn}" in
		y|Y|yes|Yes|YES) :;;
		*) exit 0;;
	esac
fi
set -x
#export_proxy
check_os
allocate_hugepages 2048
install_packages_"${distro}"
install_golang
#docker_login
# shellcheck disable=SC2119
#build_spdkimage "--force"
build_spdkcsi
#build_test_binary

vm=
if [ "${PREPARE_VM}" = yes ]; then
	allocate_hugepages 10240
	vm_build
	vm_start
	vm="vm"
	distro="fedora"
	vm "install_golang; install_docker"
	vm_copy_spdkcsi_image "--force"
	vm_copy_test_binary
fi

$vm "configure_system_${distro}"
$vm "setup_cri_dockerd"
$vm "setup_cni_networking"
$vm "stop_host_iscsid"
$vm "docker_login"

# workaround minikube permissions issues when running as root in ci(-like) env
$vm sysctl fs.protected_regular=0
$vm prepare_k8s_cluster

#prepare_spdk_storage
#prepare_xpu_node

echo "End of test environment setup!"
