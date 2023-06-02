# SPDX-License-Identifier: Apache-2.0
# Copyright (c) Arm Limited and Contributors
# Copyright (c) Intel Corporation
FROM fedora:37

ARG TAG=v23.01
ARG ARCH=native

WORKDIR /root
RUN dnf install -y git diffutils procps-ng pip kmod
RUN git clone https://github.com/spdk/spdk --branch ${TAG} --depth 1 && \
    cd spdk && git submodule update --init --depth 1 && scripts/pkgdep.sh --rdma

RUN pip3 install grpcio-tools==1.51.3 protobuf==4.22.1
RUN cd spdk && \
    ./configure --disable-tests --with-vhost --with-virtio --with-sma --with-vfio-user \
                --with-rdma --target-arch=${ARCH} && \
    make -j$(nproc)

COPY sma.yaml .
