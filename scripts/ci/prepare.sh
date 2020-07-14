#!/bin/bash -e

# prepare test environment on test agents
# - install packages and tools
# - build test images

DIR="$(dirname "$(readlink -f "$0")")"
# shellcheck source=scripts/ci/env
source "${DIR}/env"

function check_os() {
    # check distro
    distro="$(lsb_release -is)"
    if [ "${distro}" != "Ubuntu" ]; then
        echo "only supports ubuntu now"
        exit 1
    fi
    # check nvme-tcp kernel module
    if ! modprobe -n nvme-tcp; then
        echo "failed to load nvme-tcp kernel module"
        echo "upgrade kernel to 5.0+ and install linux-modules-extra package"
        exit 1
    fi
    # check iscsi_tcp kernel module
    if ! modprobe -n iscsi_tcp; then
        echo "failed to load iscsi_tcp kernel module"
        exit 1
    fi
    # check if open-iscsi is installed on host
    if dpkg -l open-iscsi > /dev/null 2>&1; then
        echo "please remove open-iscsi package on the host"
        exit 1
    fi
}

function install_packages() {
    sudo apt-get update -y
    sudo apt-get install -y make gcc curl docker.io
    sudo systemctl start docker
    # install static check tools only on x86 agent
    if [ "$(arch)" == x86_64 ]; then
        sudo apt-get install -y python3-pip
        sudo pip3 install yamllint==1.23.0 shellcheck-py==0.7.1.1
    fi
}

function install_golang() {
    if [ -d /usr/local/go ]; then
        golang_info="/usr/local/go already exists, golang install skipped"
        return
    fi
    echo "=============== installing golang ==============="
    ARCH=amd64
    if [ "$(arch)" == "aarch64" ]; then
        ARCH=arm64
    fi
    GOPKG=go${GOVERSION}.linux-${ARCH}.tar.gz
    curl -s https://dl.google.com/go/${GOPKG} | sudo tar -C /usr/local -xzf -
    /usr/local/go/bin/go version
}

function build_spdkimage() {
    if sudo docker inspect --type=image "${SPDKIMAGE}" >/dev/null 2>&1; then
        spdkimage_info="${SPDKIMAGE} image exists, build skipped"
        return
    fi
    echo "============= building spdk container =============="
    spdkdir="${ROOTDIR}/deploy/spdk"
    sudo docker build -t "${SPDKIMAGE}" -f "${spdkdir}/Dockerfile" "${spdkdir}"
}

# no official arm64 k8scsi image available, build our own
function build_arm_csi_image() {
    # XXX: must match deploy/kubernetes/controller.yaml, node.yaml
    registry=quay.io/k8scsi
    ver_provisioner=v1.4.0
    ver_attacher=v2.1.1
    ver_node_driver=v1.2.0

    godir="${HOME}/go/src/github.com/kubernetes-csi/"
    mkdir -p "${godir}"
    export PATH="/usr/local/go/bin:${PATH}"

    echo "============= building k8scsi containers =============="
    git clone https://github.com/kubernetes-csi/external-provisioner "${godir}/external-provisioner" --branch ${ver_provisioner} --depth 1
    cd "${godir}/external-provisioner"
    sed -i '/GOOS=windows/d' release-tools/build.make
    make container
    docker tag csi-provisioner:latest ${registry}/csi-provisioner:${ver_provisioner}
    docker rmi csi-provisioner:latest
    rm -rf "${godir}/external-provisioner"

    git clone https://github.com/kubernetes-csi/external-attacher "${godir}/external-attacher" --branch ${ver_attacher} --depth 1
    cd "${godir}/external-attacher"
    make container
    docker tag csi-attacher:latest ${registry}/csi-attacher:${ver_attacher}
    docker rmi csi-attacher:latest
    rm -rf "${godir}/external-attacher"

    git clone https://github.com/kubernetes-csi/node-driver-registrar "${godir}/node-driver-registrar" --branch ${ver_node_driver} --depth 1
    cd "${godir}/node-driver-registrar"
    sed -i '/GOOS=windows/d' release-tools/build.make
    make container
    docker tag csi-node-driver-registrar:latest ${registry}/csi-node-driver-registrar:${ver_node_driver}
    docker rmi csi-node-driver-registrar:latest
    rm -rf "${godir}/node-driver-registrar"
}

echo "This script is meant to run on CI nodes."
echo "It will install packages and docker images on current host."
echo "Make sure you understand what it does before going on."
read -r -p "Do you want to continue (yes/no)? " yn
case "${yn}" in
    y|Y|yes|Yes|YES) :;;
    *) exit 0;;
esac

check_os
install_packages
install_golang
build_spdkimage
[ "$(arch)" == aarch64 ] && build_arm_csi_image

echo "========================================================"
[ -n "${golang_info}" ] && echo "INFO: ${golang_info}"
[ -n "${spdkimage_info}" ] && echo "INFO: ${spdkimage_info}"
