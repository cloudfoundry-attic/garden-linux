#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)/../

source ./etc/config

cp bin/wshd $rootfs_path/sbin/wshd
chown $root_uid:$root_uid $rootfs_path/sbin/wshd
chmod 700 $rootfs_path/sbin/wshd

mkdir -p $rootfs_path/dev/pts
mount -n -t devpts -o newinstance,ptmxmode=0666 devpts $rootfs_path/dev/pts
rm -f $rootfs_path/dev/ptmx
ln -s /dev/pts/ptmx $rootfs_path/dev/ptmx
mkdir -p $rootfs_path/dev/shm
