#!/bin/bash -e

DIR="$(dirname "$(readlink -f "$0")")"

# shellcheck source=scripts/ci/env
source "${DIR}/env"
# shellcheck source=scripts/ci/common.sh
source "${DIR}/common.sh"

PROMPT_FLAG=${PROMPT_FLAG:-true}

if [[ $(id -u) != "0" ]]; then
	echo "Go away user, come back as root."
	exit 1
fi

while getopts 'yu:p:' optchar; do
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

export_proxy
check_os
allocate_hugepages
install_packages_"${distro}"
install_golang
configure_proxy
[ "${distro}" == "fedora" ] && configure_system_fedora
setup_cri_dockerd
setup_cni_networking
stop_host_iscsid
docker_login
build_spdkimage

# workaround minikube permissions issues when running as root in ci(-like) env
sysctl fs.protected_regular=0
