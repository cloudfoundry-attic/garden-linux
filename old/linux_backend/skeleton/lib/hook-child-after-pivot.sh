#!/bin/sh

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit

cd $(dirname $0)/../

. etc/config

mkdir -p /dev/pts
mount -t devpts -o newinstance,ptmxmode=0666 devpts /dev/pts
ln -sf pts/ptmx /dev/ptmx

mkdir -p /proc
mount -t proc none /proc

mkdir -p /dev/shm
mount -t tmpfs tmpfs /dev/shm

hostname $id

./bin/net-fence -containerIfcName=$network_container_iface \
                -containerIP=$network_container_ip \
                -gatewayIP=$network_host_ip \
                -subnet=$network_cidr

if [ -e /etc/seed ]; then
  . /etc/seed
fi
