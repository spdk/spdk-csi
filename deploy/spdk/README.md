# About

To test SPDKCSI, we need a working SPDK environment. This document explains how to launch SPDK and JsonRPC HTTP proxy
on localhost for function tests.

## Step by step

```bash
# build spdk container image
cd deploy/spdk
sudo docker build -t spdkdev .

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

## Single command

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
