#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)/../

source ./etc/config

mkdir -p $rootfs_path/sbin
cp bin/wshd $rootfs_path/sbin/wshd
cp lib/hook $rootfs_path/sbin/hook
cp etc/config $rootfs_path/etc/config
chown $root_uid:$root_uid $rootfs_path/sbin/wshd
chown $root_uid:$root_uid $rootfs_path/sbin/hook
chown $root_uid:$root_uid $rootfs_path/etc/config
chmod 700 $rootfs_path/sbin/wshd

mkdir -p $rootfs_path/dev/pts
mount -n -t devpts -o newinstance,ptmxmode=0666 devpts $rootfs_path/dev/pts
ln -s /dev/pts/ptmx $rootfs_path/dev/ptmx
chown $root_uid:$root_uid $rootfs_path/dev/ptmx

# hack to ensure proc is owned by correct user until translation
mkdir -p $rootfs_path/proc
chown $root_uid:$root_uid $rootfs_path/proc
chmod 777 /tmp
