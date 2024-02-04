#!/bin/bash

if [ "$(id -u)" -ne 0 ]; then
    echo "This script requires sudo privileges. Please enter your password:"
    exec sudo "$0" "$@" # This re-executes the script with sudo
fi

echo "======= setting huge pages ======="
echo "vm.nr_hugepages=2048" >>/etc/sysctl.conf
sysctl -p

# confirm it by running
cat /proc/meminfo | grep -i hug

echo "======= creating huge pages mount ======="
mkdir /mnt/huge
mount -t hugetlbfs -o size=2G nodev /mnt/huge
echo "nodev /mnt/huge hugetlbfs size=2G 0 0" >>/etc/fstab

## TODO: detect the present of additional NVMe Volume
