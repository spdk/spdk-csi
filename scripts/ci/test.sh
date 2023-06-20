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
  -x              Exclude running xPU tests."
    echo "$usage" >&2
}

echo "Running script as user: $(whoami)"

# Default run all xPU tests on amd64 hosts.
# Invoke this scirpt with -x if to exclude xPU tets
RUN_XPU_TESTS=$(! [ "${ARCH}" = "amd64" ]; echo $?)
for arg in "$@" ; do
  case "$arg" in
  -h|--help) Usage ; exit ;;
  -x) unset RUN_XPU_TESTS ;;
  *) echo "Ignoring unknown argument: $arg" >&2 ;;
  esac
  shift
done

trap on_exit EXIT ERR

unit_test
set -x
if [ "$RUN_XPU_TESTS" ] ; then
    echo "Running E2E tests in VM"
    # ./scripts/ci/prepare.sh is run with sudo user. Where
    # the ssh key generated is owned by root, hence make it
    # accessbile by the current user
    perm="$(id -u):$(id -g)"
    sudo chown "$perm" "${WORKERDIR}"/id_rsa

    vm e2e_test "-xpu=true"
    vm "make -C \${ROOTDIR} helm-test HELM_SKIP_SPDKDEV_CHECK=1"
    vm_stop
else
    e2e_test "-xpu=false"
    helm_test
fi

set +x
exit 0
