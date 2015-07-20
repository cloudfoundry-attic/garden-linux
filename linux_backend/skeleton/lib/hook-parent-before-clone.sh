#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)/../

source ./etc/config

cp etc/config $rootfs_path/etc/config
chown $root_uid:$root_uid $rootfs_path/etc/config

mkdir -p $rootfs_path/dev/pts
ln -s /dev/pts/ptmx $rootfs_path/dev/ptmx
chown -h $root_uid:$root_uid $rootfs_path/dev/ptmx

# hack to ensure proc is owned by correct user until translation
mkdir -p $rootfs_path/proc
chown $root_uid:$root_uid $rootfs_path/proc
chmod 777 /tmp
