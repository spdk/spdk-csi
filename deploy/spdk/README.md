# About

To test SPDKCSI, we need working SPDK environments.
This document explains how to launch SPDK storage nodes with JsonRPC HTTP proxy,
and SPDK xPU nodes with SMA server on localhost for function tests.

```bash
# build spdk container image, which could be used for SPDK storage node and xPU node
cd deploy/spdk
sudo docker build -t spdkdev .
```

## Step by step to launch a SPDK storage node

```bash
# allocate 2G hugepages (1024*2M)
sudo sh -c 'echo 1024 > /proc/sys/vm/nr_hugepages'

# start spdk target
sudo docker run -it --rm --name spdkdev --privileged --net host \
-v /dev/hugepages:/dev/hugepages -v /dev/shm:/dev/shm spdkdev /root/spdk/build/bin/spdk_tgt
# run below commands in another console

# create 1G malloc bdev
sudo docker exec -it spdkdev /root/spdk/scripts/rpc.py bdev_malloc_create -b Malloc0 1024 4096

# create lvstore
sudo docker exec -it spdkdev /root/spdk/scripts/rpc.py bdev_lvol_create_lvstore Malloc0 lvs0

# start jsonrpc http proxy on 127.0.0.1:9009
sudo docker exec -it spdkdev /root/spdk/scripts/rpc_http_proxy.py 127.0.0.1 9009 spdkcsiuser spdkcsipass
```

## Single command to launch a SPDK storage node

Combine above steps to a single command can be convenient. But it's harder to debug if error happens.

```bash
sudo docker run -it --rm --privileged --net host \
  -v /dev/hugepages:/dev/hugepages -v /dev/shm:/dev/shm -v /proc:/proc \
  spdkdev sh -c 'echo 1024 > /proc/sys/vm/nr_hugepages && \
                 /root/spdk/build/bin/spdk_tgt > /tmp/spdk-tgt.log 2>&1 & \
                 echo wait 5s... && sleep 5s && cd /root/spdk/scripts && \
                 ./rpc.py bdev_malloc_create -b Malloc0 1024 4096 && \
                 ./rpc.py bdev_lvol_create_lvstore Malloc0 lvs0 && \
                 ./rpc_http_proxy.py 127.0.0.1 9009 spdkcsiuser spdkcsipass'
```

## Step by step to launch a SPDK xPU node with SMA

```bash
# start spdkdev_sma container
sudo docker run -it --rm --name spdkdev_sma --privileged --net host -v /dev/hugepages:/dev/hugepages \
-v /dev/shm:/dev/shm -v /var/tmp:/var/tmp -v /lib/modules:/lib/modules spdkdev

# run below commands in another console
# start spdk target
sudo docker exec -id spdkdev_sma sh -c \
"/root/spdk/scripts/setup.sh; /root/spdk/build/bin/spdk_tgt -S /var/tmp -m 0x3 &"

# start sma server
sudo docker exec -id spdkdev_sma sh -c "/root/spdk/scripts/sma.py --config /root/sma.yaml"
```
