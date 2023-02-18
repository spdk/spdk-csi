#!/bin/bash -e

# build and test spdkcsi, can be invoked manually or by jenkins

DIR="$(dirname "$(readlink -f "$0")")"
# shellcheck source=scripts/ci/env
source "${DIR}/env"
# shellcheck source=scripts/ci/common.sh
source "${DIR}/common.sh"

export_proxy
docker_login
build_spdkcsi
trap cleanup EXIT
prepare_k8s_cluster
prepare_spdk
prepare_sma
unit_test
e2e_test
helm_test
