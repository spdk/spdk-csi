#!/bin/bash -e

# This common.sh contains all the functions needed for all the e2e tests
# including, configuring proxies, installing packages and tools, building test images, etc.

# FIXME (JingYan): too many "echo"s, try to define a logger function with different logging levels, including
# "info", "warning" and "error", etc. and replace all the "echo" with the logger function

SPDK_CONTAINER="spdkdev-e2e"
SPDK_SMA_CONTAINER="spdkdev-sma"

function export_proxy() {
	local http_proxies

	http_proxies=$(env | { grep -Pi "http[s]?_proxy" || true; })
	[ -z "$http_proxies" ] && return 0

	for proxy in $http_proxies; do
		# shellcheck disable=SC2001,SC2005
		echo "$(sed "s/.*=/\U&/" <<< "$proxy")"
		# shellcheck disable=SC2001
		export "$(sed "s/.*=/\U&/" <<< "$proxy")"
	done

	export NO_PROXY="$NO_PROXY,127.0.0.1,localhost,10.0.0.0/8,192.168.0.0/16,.internal"
	export no_proxy="$no_proxy,127.0.0.1,localhost,10.0.0.0/8,192.168.0.0/16,.internal"
}

function check_os() {
	# check ARCH
	ARCH=$(arch)
	if [[ "$(arch)" == "x86_64" ]]; then
		ARCH="amd64"
	elif [[ "$(arch)" == "aarch64" ]]; then
		ARCH="arm64"
	else
		echo "${ARCH} is not supported"
		exit 1
	fi
	export ARCH

	# check distro
	source /etc/os-release
	case $ID in
	fedora)
		distro="fedora"
		;;
	debian)
		echo "Warning: Debian is not officially supported, using Ubuntu setup"
		distro="ubuntu"
		;;
	ubuntu)
		distro="ubuntu"
		;;
	*)
		echo "Only supports Ubuntu and Fedora now"
		exit 1
		;;
	esac
	export distro

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
	# check vfio-pci kernel module
	if ! modprobe -n vfio-pci; then
		echo "failed to load vfio-pci kernel module"
		exit 1
	fi
}

# allocate 2048*2M hugepages for prepare_spdk() and prepare_sma()
function allocate_hugepages() {
	local HUGEPAGES_MIN=2048
	local NR_HUGEPAGES=/proc/sys/vm/nr_hugepages
	if [[ -f ${NR_HUGEPAGES} ]]; then
		if [[ $(< ${NR_HUGEPAGES}) -lt ${HUGEPAGES_MIN} ]]; then
			echo ${HUGEPAGES_MIN} > ${NR_HUGEPAGES} || true
		fi
		echo "/proc/sys/vm/nr_hugepages: $(< ${NR_HUGEPAGES})"
		if [[ $(< ${NR_HUGEPAGES}) -lt ${HUGEPAGES_MIN} ]]; then
			echo allocating ${HUGEPAGES_MIN} hugepages failed
			exit 1
		fi
	fi
	cat /proc/meminfo
}

function install_packages_ubuntu() {
	apt-get update -y
	apt-get install -y make \
					gcc \
					curl \
					docker.io \
					conntrack \
					socat \
					wget
	systemctl start docker
	# install static check tools only on x86 agent
	if [ "$(arch)" == x86_64 ]; then
		apt-get install -y python3-pip
		pip3 install yamllint==1.23.0 shellcheck-py==0.7.1.1
	fi
}

function install_packages_fedora() {
	dnf check-update || true
	dnf install -y make \
					gcc \
					curl \
					conntrack \
					bind-utils \
					socat \
					wget
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
		echo "========================================================"
		[ -n "${golang_info}" ] && echo "INFO: ${golang_info}"
		return
	fi
	echo "=============== installing golang ==============="
	GOPKG=go${GOVERSION}.linux-${ARCH}.tar.gz
	curl -s https://dl.google.com/go/"${GOPKG}" | tar -C /usr/local -xzf -
	/usr/local/go/bin/go version
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
	sed -e "s:^\(no_proxy\)=.*:\1=${NO_PROXY}:gI" -i /etc/environment
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

	# check if open-iscsi is installed on host
	iscsi_check_cmd="rpm --quiet -q iscsi-initiator-utils"
	iscsi_remove_cmd="dnf remove -y iscsi-initiator-utils"
	if $iscsi_check_cmd; then
		$iscsi_remove_cmd || true
	fi
}

function setup_cri_dockerd() {
	# use the cri-dockerd adapter to integrate Docker Engine with Kubernetes 1.24 or higher version
	local STATUS
	STATUS="$(systemctl is-active cri-docker.service || true)"
	if [ "${STATUS}" == "active" ]; then
		cri_dockerd_info="cri-docker is already active, cri-dockerd setup skipped"
		echo "========================================================"
		[ -n "${cri_dockerd_info}" ] && echo "INFO: ${cri_dockerd_info}"
		return
	fi

	echo "=============== setting up cri-dockerd ==============="
	echo "=== downloading cri-dockerd-${CRIDOCKERD_VERSION}"
	wget -c https://github.com/Mirantis/cri-dockerd/releases/download/v"${CRIDOCKERD_VERSION}"/cri-dockerd-"${CRIDOCKERD_VERSION}"."${ARCH}".tgz -O - | tar -xz -C /usr/local/bin/ --strip-components 1
	wget https://raw.githubusercontent.com/Mirantis/cri-dockerd/master/packaging/systemd/cri-docker.service -P /etc/systemd/system/
	wget https://raw.githubusercontent.com/Mirantis/cri-dockerd/master/packaging/systemd/cri-docker.socket -P /etc/systemd/system/

	# start cri-docker service
	sed -i -e 's,/usr/bin/cri-dockerd,/usr/local/bin/cri-dockerd,' /etc/systemd/system/cri-docker.service
	systemctl daemon-reload
	systemctl enable cri-docker.service
	systemctl enable --now cri-docker.socket

	echo "=== downloading crictl-${CRITOOLS_VERSION}"
	wget -c https://github.com/kubernetes-sigs/cri-tools/releases/download/"${CRITOOLS_VERSION}"/crictl-"${CRITOOLS_VERSION}"-linux-"${ARCH}".tar.gz -O - | tar -xz -C /usr/local/bin/
}

function setup_cni_networking() {
	echo "=============== setting up CNI networking ==============="
	echo "=== downloading 10-crio-bridge.conf and CNI plugins"
	mkdir -p /etc/cni/net.d
	wget https://raw.githubusercontent.com/cri-o/cri-o/v1.23.4/contrib/cni/10-crio-bridge.conf -P /etc/cni/net.d/
	mkdir -p /opt/cni/bin
	wget -c https://github.com/containernetworking/plugins/releases/download/"${CNIPLUGIN_VERSION}"/cni-plugins-linux-"${ARCH}"-"${CNIPLUGIN_VERSION}".tgz -O - | tar -xz -C /opt/cni/bin
}

function stop_host_iscsid() {
	local STATUS
	STATUS="$(systemctl is-enabled iscsid.service >&/dev/null || true)"
	if [ "${STATUS}" == "enabled" ]; then
		systemctl disable iscsid.service
		systemctl disable iscsid.socket
	fi

	STATUS="$(systemctl is-active iscsid.service >&/dev/null || true)"
	if [ "${STATUS}" == "active" ]; then
		systemctl stop iscsid.service
		systemctl stop iscsid.socket
	fi
}

function docker_login {
	if [[ -n "$DOCKERHUB_USER" ]] && [[ -n "$DOCKERHUB_SECRET" ]]; then
		docker login --username "$DOCKERHUB_USER" \
			--password-stdin <<< "$(cat "$DOCKERHUB_SECRET")"
	fi
}

function build_spdkimage() {
	if docker inspect --type=image "${SPDKIMAGE}" >/dev/null 2>&1; then
		spdkimage_info="${SPDKIMAGE} image exists, build skipped"
		echo "========================================================"
		[ -n "${spdkimage_info}" ] && echo "INFO: ${spdkimage_info}"
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

function build_spdkcsi() {
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

function prepare_k8s_cluster() {
	echo "======== prepare k8s cluster with minikube ========"
	sudo modprobe iscsi_tcp
	sudo modprobe nvme-tcp
	sudo modprobe vfio-pci
	export KUBE_VERSION MINIKUBE_VERSION
	sudo --preserve-env HOME="$HOME" "${ROOTDIR}/scripts/minikube.sh" up
}

# FIXME (JingYan): after starting the container, instead of waiting for a fixed number of seconds before executing commands
# in the container in the prepare_spdk() and prepare_sma() functions, we could try to do docker exec here and call spdk's rpc.py
# to try communicating with target. See https://github.com/spdk/spdk/blob/master/test/common/autotest_common.sh#L785

function prepare_spdk() {
	echo "======== start spdk target for storage node ========"
	grep Huge /proc/meminfo
	# start spdk target for storage node
	sudo docker run -id --name "${SPDK_CONTAINER}" --privileged --net host -v /dev/hugepages:/dev/hugepages -v /dev/shm:/dev/shm "${SPDKIMAGE}" /root/spdk/build/bin/spdk_tgt
	sleep 20s
	# wait for spdk target ready
	sudo docker exec -i "${SPDK_CONTAINER}" timeout 5s /root/spdk/scripts/rpc.py framework_wait_init
	# create 1G malloc bdev
	sudo docker exec -i "${SPDK_CONTAINER}" /root/spdk/scripts/rpc.py bdev_malloc_create -b Malloc0 1024 4096
	# create lvstore
	sudo docker exec -i "${SPDK_CONTAINER}" /root/spdk/scripts/rpc.py bdev_lvol_create_lvstore Malloc0 lvs0
	# start jsonrpc http proxy
	sudo docker exec -id "${SPDK_CONTAINER}" /root/spdk/scripts/rpc_http_proxy.py "${JSONRPC_IP}" "${JSONRPC_PORT}" "${JSONRPC_USER}" "${JSONRPC_PASS}"
	sleep 10s
}

function prepare_sma() {
	echo "======== start spdk target for IPU node ========"
	# start spdk target for IPU node
	sudo docker run -id --name "${SPDK_SMA_CONTAINER}" --privileged --net host -v /dev/hugepages:/dev/hugepages -v /dev/shm:/dev/shm -v /var/tmp:/var/tmp -v /lib/modules:/lib/modules "${SPDKIMAGE}"
	sudo docker exec -i "${SPDK_SMA_CONTAINER}" sh -c "HUGEMEM=2048 /root/spdk/scripts/setup.sh; /root/spdk/build/bin/spdk_tgt -S /var/tmp -m 0x3 &"
	sleep 20s
	echo "======== start sma server ========"
	# start sma server
	sudo docker exec -d "${SPDK_SMA_CONTAINER}" sh -c "/root/spdk/scripts/sma.py --config /root/sma.yaml"
	sleep 10s
}

function unit_test() {
	echo "======== run unit test ========"
	make -C "${ROOTDIR}" test
}

function e2e_test() {
	echo "======== run E2E test ========"
	export PATH="/var/lib/minikube/binaries/${KUBE_VERSION}:${PATH}"
	make -C "${ROOTDIR}" e2e-test
}

function helm_test() {
	sudo docker rm -f "${SPDK_CONTAINER}" > /dev/null || :
	sudo docker rm -f "${SPDK_SMA_CONTAINER}" > /dev/null || :
	make -C "${ROOTDIR}" helm-test
}

function cleanup() {
	sudo docker stop "${SPDK_CONTAINER}" || :
	sudo docker rm -f "${SPDK_CONTAINER}" > /dev/null || :
	sudo docker stop "${SPDK_SMA_CONTAINER}" || :
	sudo docker rm -f "${SPDK_SMA_CONTAINER}" > /dev/null || :
	sudo --preserve-env HOME="$HOME" "${ROOTDIR}/scripts/minikube.sh" clean || :
	# TODO: remove dangling nvmf,iscsi disks
}