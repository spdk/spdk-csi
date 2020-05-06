FROM fedora:31

ARG TAG=v20.01
ARG ARCH=native

WORKDIR /root
RUN dnf install -y git
RUN git clone https://github.com/spdk/spdk --branch ${TAG} --depth 1 && \
    cd spdk && git submodule update --init --depth 1 && scripts/pkgdep.sh
RUN cd spdk && \
    ./configure --disable-tests --without-vhost --without-virtio \
                --with-rdma --target-arch=${ARCH} && \
    make
