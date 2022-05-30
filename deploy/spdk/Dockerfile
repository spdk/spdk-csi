# SPDX-License-Identifier: Apache-2.0
# Copyright (c) Arm Limited and Contributors
# Copyright (c) Intel Corporation
FROM fedora:33

ARG TAG=v21.04
ARG ARCH=native

WORKDIR /root
RUN dnf install -y git diffutils procps-ng pip
RUN git clone https://github.com/spdk/spdk --branch ${TAG} --depth 1 && \
    cd spdk && git submodule update --init --depth 1 && scripts/pkgdep.sh --rdma
RUN cd spdk && \
    ./configure --disable-tests --without-vhost --without-virtio \
                --with-rdma --target-arch=${ARCH} && \
    make
