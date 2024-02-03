#!/bin/bash

sudo vi /etc/sysctl.conf
vm.nr_hugepages=2048
sudo sysctl -p
sudo sysctl -w vm.nr_hugepages=2048

# confirm it by running
cat /proc/meminfo | grep -i hug

sudo mkdir /mnt/huge
sudo mount -t hugetlbfs -o size=2G nodev /mnt/huge

sudo vi /etc/fstab
nodev /mnt/huge hugetlbfs size=2G 0 0

kubectl taint nodes <node-name >tag=sbcache:NoSchedule

# make sure that port 5000 is available
