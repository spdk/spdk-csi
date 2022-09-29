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

    if [[ "${distro}" == *"debian"* ]]; then
        echo "Debian detected"
        distro=ubuntu
    fi

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

function setup_cri_dockerd() {
    # use the cri-dockerd adapter to integrate Docker Engine with Kubernetes 1.24 or higher version
    local STATUS
    STATUS="$(systemctl is-active cri-docker.service || true)"
    if [ "${STATUS}" == "active" ]; then
        cri_dockerd_info="cri_docker is already active, cri_dockerd setup skipped"
        return
    fi

    echo "=============== setting up cri_dockerd ==============="
    local ARCH
    ARCH=$(arch)
    if [[ "$(arch)" == "x86_64" ]]; then
        ARCH="amd64"
    elif [[ "$(arch)" == "aarch64" ]]; then
        ARCH="arm64"
    else
        echo "${ARCH} is not supported"
        exit 1
    fi

    echo "=== downloading cri_dockerd-${CRIDOCKERD_VERSION}"
    wget -qO- https://github.com/Mirantis/cri-dockerd/releases/download/v"${CRIDOCKERD_VERSION}"/cri-dockerd-"${CRIDOCKERD_VERSION}"."${ARCH}".tgz | tar xvz -C /tmp
    wget -P /tmp https://raw.githubusercontent.com/Mirantis/cri-dockerd/master/packaging/systemd/cri-docker.service
    wget -P /tmp/ https://raw.githubusercontent.com/Mirantis/cri-dockerd/master/packaging/systemd/cri-docker.socket
    sudo mv /tmp/cri-dockerd/cri-dockerd /usr/local/bin/
    sudo mv /tmp/cri-docker.service /etc/systemd/system/
    sudo mv /tmp/cri-docker.socket /etc/systemd/system/

    # start cri-docker service
    sudo sed -i -e 's,/usr/bin/cri-dockerd,/usr/local/bin/cri-dockerd,' /etc/systemd/system/cri-docker.service
    systemctl daemon-reload
    systemctl enable cri-docker.service
    systemctl enable --now cri-docker.socket

    echo "=== downloading crictl-${CRIDOCKERD_VERSION}"
    wget -qO- https://github.com/kubernetes-sigs/cri-tools/releases/download/"${KUBE_VERSION}"/crictl-"${KUBE_VERSION}"-linux-"${ARCH}".tar.gz | tar xvz -C /tmp
    sudo mv /tmp/crictl /usr/local/bin/
}

function setup_cni_networking() {
    echo "=============== setting up CNI networking ==============="
    local ARCH
    ARCH=$(arch)
    if [[ "$(arch)" == "x86_64" ]]; then
        ARCH="amd64"
    elif [[ "$(arch)" == "aarch64" ]]; then
        ARCH="arm64"
    else
        echo "${ARCH} is not supported"
        exit 1
    fi

    echo "=== downloading 10-crio-bridge.conf and CNI plugins"
    wget -P /tmp https://raw.githubusercontent.com/cri-o/cri-o/main/contrib/cni/10-crio-bridge.conf
    mkdir -p /tmp/plugins
    mkdir -p /etc/cni/net.d
    wget -qO- https://github.com/containernetworking/plugins/releases/download/"${CNIPLUGIN_VERSION}"/cni-plugins-linux-"${ARCH}"-"${CNIPLUGIN_VERSION}".tgz | tar xvz -C /tmp/plugins
    sudo mv /tmp/10-crio-bridge.conf /etc/cni/net.d/
    sudo mkdir -p /opt/cni/bin
    sudo mv /tmp/plugins/* /opt/cni/bin/
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

function stop_iscsid_locally() {
    # iscsid service is enabled and started locally by default on some OSs
    # which will result in iscsid can not start inside containers, and then spdk iscsi e2e test failed
    # o top and disable iscsid service locally avoid possible conflicts
    local STATUS
    STATUS="$(systemctl is-enabled iscsid.service || true)"
    if [ "${STATUS}" == "enabled" ]; then
	    systemctl disable iscsid.service
    fi

    STATUS="$(systemctl is-active iscsid.service || true)"
    if [ "${STATUS}" == "active" ]; then
	    systemctl stop iscsid.service
            systemctl stop iscsid.socket
    fi
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
setup_cri_dockerd
setup_cni_networking
stop_iscsid_locally
docker_login
build_spdkimage

echo "========================================================"
[ -n "${golang_info}" ] && echo "INFO: ${golang_info}"
[ -n "${cri_dockerd_info}" ] && echo "INFO: ${cri_dockerd_info}"
[ -n "${spdkimage_info}" ] && echo "INFO: ${spdkimage_info}"
