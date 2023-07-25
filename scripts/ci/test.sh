#!/bin/bash -e

# build and test spdkcsi, can be invoked manually or by jenkins

DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# shellcheck source=scripts/ci/env
source "${DIR}/env"
# shellcheck source=scripts/ci/common.sh
source "${DIR}/common.sh"

Usage() {
    usage="Usage: $0 [option]
Run all SPDK-CSI tests
Options:
  -h, --help      display this help and exit
  -x              Exclude xPU tests which require a VM to be run"
    echo "$usage" >&2
}

echo "Running script as user: $(whoami)"

# Default run all xPU tests on amd64 hosts.
# Invoke this script with -x if to exclude xPU tets
if [ "${ARCH}" = amd64 ]; then
	RUN_XPU_VM_TESTS=yes
else
	RUN_XPU_VM_TESTS=no
fi

for arg in "$@" ; do
  case "$arg" in
  -h|--help) Usage ; exit ;;
  -x) RUN_XPU_VM_TESTS=no ;;
  *) echo "Ignoring unknown argument: $arg" >&2 ;;
  esac
  shift
done

trap on_exit EXIT ERR

unit_test
set -x
perm="$(id -u):$(id -g)"
if [ "${RUN_XPU_VM_TESTS}" = yes ]; then
    echo "Running E2E tests in VM"
    # ./scripts/ci/prepare.sh is run with sudo user. Where
    # the ssh key generated is owned by root, hence make it
    # accessible by the current user
    sudo chown "$perm" "${WORKERDIR}"/id_rsa

    vm e2e_test
    vm "make -C \${ROOTDIR} helm-test HELM_SKIP_SPDKDEV_CHECK=1"
    vm_stop
else
    # As minikube is installed by the root user, it is necessary to copy the
    # authentication information to a regular user
    sudo cp -r /root/.kube /root/.minikube "${HOME}"
    sudo chown -R "$perm" "${HOME}"/.kube "${HOME}"/.minikube
    sed -i "s#/root/#$HOME/#g" "${HOME}"/.kube/config
    e2e_test --ginkgo.label-filter="!xpu-vm-tests" # exclude tests labeled with xpu-vm-tests
    helm_test
fi
set +x
exit 0
