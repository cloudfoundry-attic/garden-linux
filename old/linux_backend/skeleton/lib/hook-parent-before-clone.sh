#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)/../

source ./etc/config

cp bin/wshd $rootfs_path/sbin/wshd
chown $user_uid:$user_uid $rootfs_path/sbin/wshd
chmod 700 $rootfs_path/sbin/wshd

mkdir -p $rootfs_path/dev/pts
mount -n -t devpts -o newinstance,ptmxmode=0666 devpts $rootfs_path/dev/pts
mkdir -p $rootfs_path/proc
mount -n -t proc none $rootfs_path/proc
mkdir -p $rootfs_path/dev/shm
mount -n -t tmpfs -o nodev tmpfs $rootfs_path/dev/shm
