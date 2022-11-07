#!/bin/bash -e

# prepare test environment on test agents
# - install packages and tools
# - build test images

DIR="$(dirname "$(readlink -f "$0")")"
# shellcheck source=scripts/ci/env
source "${DIR}/env"
# shellcheck source=scripts/ci/common.sh
source "${DIR}/common.sh"
PROMPT_FLAG=true

function check_os() {
    # check distro
    source /etc/os-release
    distro=${NAME,,}

    if [ "${distro}" != "ubuntu" ] && [ "${distro}" != "fedora" ]; then
        echo "Only supports Ubuntu and Fedora now"
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
    iscsi_check_cmd="dpkg -l open-iscsi > /dev/null 2>&1"
    if [[ "${distro}" == "fedora" ]]; then
        iscsi_check_cmd="rpm --quiet -q iscsi-initiator-utils"
    fi
    if $iscsi_check_cmd; then
        echo "please remove open-iscsi package on the host"
        exit 1
    fi
}

function install_packages_ubuntu() {
    apt-get update -y
    apt-get install -y make gcc curl docker.io conntrack wget
    systemctl start docker
    # install static check tools only on x86 agent
    if [ "$(arch)" == x86_64 ]; then
        apt-get install -y python3-pip
        pip3 install yamllint==1.23.0 shellcheck-py==0.7.1.1
    fi
}

function install_packages_fedora() {
    dnf check-update || true
    dnf install -y make gcc curl conntrack bind-utils socat wget

    if ! hash docker &> /dev/null; then
        dnf remove -y docker*
        dnf install -y dnf-plugins-core
        dnf config-manager --add-repo \
            https://download.docker.com/linux/fedora/docker-ce.repo
        dnf check-update || true
        dnf install -y docker-ce docker-ce-cli containerd.io
    fi
    systemctl start docker

    # install static check tools only on x86 agent
    if [ "$(arch)" == x86_64 ]; then
        dnf install -y python3-pip
        pip3 install yamllint==1.23.0 shellcheck-py==0.7.1.1
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
    curl -s https://dl.google.com/go/${GOPKG} | tar -C /usr/local -xzf -
    /usr/local/go/bin/go version
}

function build_spdkimage() {
    if docker inspect --type=image "${SPDKIMAGE}" >/dev/null 2>&1; then
        spdkimage_info="${SPDKIMAGE} image exists, build skipped"
        return
    fi

    if [ -n "$HTTP_PROXY" ] && [ -n "$HTTPS_PROXY" ]; then
        docker_proxy_opt=("--build-arg" "http_proxy=$HTTP_PROXY" "--build-arg" "https_proxy=$HTTPS_PROXY")
    fi

    echo "============= building spdk container =============="
    spdkdir="${ROOTDIR}/deploy/spdk"
    docker build -t "${SPDKIMAGE}" -f "${spdkdir}/Dockerfile" \
    "${docker_proxy_opt[@]}" "${spdkdir}" && spdkimage_info="${SPDKIMAGE} image build successfully."
}

function configure_proxy() {
    if [ -n "${DOCKER_MIRROR}" ]; then
        mkdir -p /etc/docker
        cat <<EOF > /etc/docker/daemon.json
{
  "insecure-registries": [
    "${DOCKER_MIRROR}"
  ],
  "registry-mirrors": [
    "https://${DOCKER_MIRROR}"
  ]
}
EOF
    fi
    mkdir -p /etc/systemd/system/docker.service.d
    cat <<- EOF > /etc/systemd/system/docker.service.d/http-proxy.conf
[Service]
Environment="HTTP_PROXY=$HTTP_PROXY"
Environment="HTTPS_PROXY=$HTTPS_PROXY"
Environment="NO_PROXY=$NO_PROXY"
EOF
    systemctl daemon-reload
    systemctl restart docker
}

function configure_system_fedora() {
    # Make life easier and set SE Linux to Permissive if it's
    # not already disabled.
    [ "$(getenforce)" != "Disabled" ] && setenforce "Permissive"

    # Disable swap memory so that minikube does not complain.
    # On recent Fedora systemd releases also remove zram tools
    # to keep swap from regenerating.
    if rpm -q --quiet systemd; then
        dnf remove -y zram*
    fi
    swapoff -a
}

function docker_login {
    if [[ -n "$DOCKERHUB_USER" ]] && [[ -n "$DOCKERHUB_SECRET" ]]; then
        docker login --username "$DOCKERHUB_USER" \
            --password-stdin <<< "$(cat "$DOCKERHUB_SECRET")"
    fi
}

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
install_packages_"${distro}"
install_golang
configure_proxy
[ "${distro}" == "fedora" ] && configure_system_fedora
docker_login
build_spdkimage

echo "========================================================"
[ -n "${golang_info}" ] && echo "INFO: ${golang_info}"
[ -n "${spdkimage_info}" ] && echo "INFO: ${spdkimage_info}"
