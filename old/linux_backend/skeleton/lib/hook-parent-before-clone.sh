#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)/../

source ./etc/config

cp bin/wshd $rootfs_path/sbin/wshd
chmod 700 $rootfs_path/sbin/wshd
