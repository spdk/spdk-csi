#!/bin/bash -e

KUBE_VERSION=${KUBE_VERSION:-v1.25.0}
MINIKUBE_VERSION=${MINIKUBE_VERSION:-v1.27.0}
MINIKUBE_DRIVER=${MINIKUBE_DRIVER:-none}
MINIKUBE_ARCH=amd64
if [ "$(uname -m)" = "aarch64" ]; then
	MINIKUBE_ARCH=arm64
fi

function install_minikube() {
	if hash minikube 2> /dev/null; then
		version=$(minikube version | awk '{print $3; exit;}')
		if [[ "${version}" != "${MINIKUBE_VERSION}" ]]; then
			echo "installed minikube doesn't match requested version ${MINIKUBE_VERSION}"
			echo "please remove minikube ${version} first"
			exit 1
		fi
		echo "minikube-${version} already installed"
		return
	fi

	echo "=== downloading minikube-${MINIKUBE_VERSION}"
	curl -Lo /usr/local/bin/minikube https://storage.googleapis.com/minikube/releases/"${MINIKUBE_VERSION}"/minikube-linux-"${MINIKUBE_ARCH}" && chmod +x /usr/local/bin/minikube
}

case "$1" in
	up)
		install_minikube
		echo "=== starting minikube with kubeadm bootstrapper"
		CHANGE_MINIKUBE_NONE_USER=true minikube start -b kubeadm --kubernetes-version="${KUBE_VERSION}" --vm-driver="${MINIKUBE_DRIVER}" --alsologtostderr -v=5
		;;
	down)
		minikube stop
		;;
	clean)
		minikube delete
		;;
	*)
		echo "$0 [up|down|clean]"
		;;
esac
