#!/bin/bash -e

# build and test spdkcsi, can be invoked manually or by jenkins

DIR="$(dirname "$(readlink -f "$0")")"
# shellcheck source=scripts/ci/env
source "${DIR}/env"

SPDK_CONTAINER="spdkdev-${RANDOM}"

function build() {
    export PATH="/usr/local/go/bin:${PATH}"
    make clean
    echo "======== build spdkcsi ========"
    make -C "${ROOTDIR}" spdkcsi
    if [ "$(arch)" == "x86_64" ]; then
        echo "======== static check ========"
        make -C "${ROOTDIR}" lint
    fi
    echo "======== build container ========"
    # XXX: should match image name:tag in Makefile
    sudo docker rmi spdkcsi/spdkcsi:canary > /dev/null || :
    sudo --preserve-env=PATH,HOME make -C "${ROOTDIR}" image
}

function prepare_spdk() {
    echo "======== start spdk target ========"
    # allocate 1024*2M hugepage
    sudo sh -c 'echo 1024 > /proc/sys/vm/nr_hugepages'
    # start spdk target
    sudo docker run -id --name "${SPDK_CONTAINER}" --privileged --net host -v /dev/hugepages:/dev/hugepages -v /dev/shm:/dev/shm ${SPDKIMAGE} /root/spdk/app/spdk_tgt/spdk_tgt
    sleep 20s
    # wait for spdk target ready
    sudo docker exec -i "${SPDK_CONTAINER}" timeout 5s /root/spdk/scripts/rpc.py framework_wait_init
    # create 1G malloc bdev
    sudo docker exec -i "${SPDK_CONTAINER}" /root/spdk/scripts/rpc.py bdev_malloc_create -b Malloc0 1024 4096
    # create lvstore
    sudo docker exec -i "${SPDK_CONTAINER}" /root/spdk/scripts/rpc.py bdev_lvol_create_lvstore Malloc0 lvs0
    # start jsonrpc http proxy
    sudo docker exec -id "${SPDK_CONTAINER}" /root/spdk/scripts/rpc_http_proxy.py ${JSONRPC_IP} ${JSONRPC_PORT} ${JSONRPC_USER} ${JSONRPC_PASS}
    sleep 10s
}

function unit_test() {
    echo "======== run unit test ========"
    make -C "${ROOTDIR}" test
}

function e2e_test() {
    echo "======== E2E test not implemented yet ========"
    # TODO: modprobe nvme-tcp, minikube up, run e2e tests
}

function cleanup() {
    sudo docker rm -f "${SPDK_CONTAINER}" > /dev/null || :
    # TODO: minikube clean, remove dangling nvmf,iscsi disks
}

build
trap cleanup EXIT
prepare_spdk
unit_test
e2e_test
