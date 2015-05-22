#!/bin/bash

set -e
set -x

if [ $(whoami) != "root" ]
then
    echo "Must be run as root" >&2
    exit 1
fi

if dpkg -l | grep xfsprogs
then
    echo "xfsprogs already installed"
else
    apt-get update
    apt-get install -y xfsprogs
fi

backing_store=/opt/xfs_backing_store
loopback_device=/dev/xfs_loop
mount_point=/tmp/xfs_mount
if [ ! -f $backing_store ]
then
    dd if=/dev/zero of=$backing_store bs=1M count=3000
    mknod $loopback_device b 7 200
    losetup $loopback_device $backing_store
    mkfs.xfs $backing_store
    mkdir -p $mount_point
    mount $loopback_device $mount_point
else
    echo "xfs mount already set up, skipping"
fi
