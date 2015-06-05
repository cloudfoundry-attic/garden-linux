#!/bin/bash

set -e
set -x

if [ $(whoami) != "root" ]
then
    echo "Must be run as root" >&2
    exit 1
fi

backing_store=/opt/btrfs_backing_store
loopback_device=/dev/btrfs_loop
mount_point=/tmp/btrfs_mount

if [ ! -d $mount_point ]
then
    dd if=/dev/zero of=$backing_store bs=1M count=3000
    mknod $loopback_device b 7 200
    losetup $loopback_device $backing_store
    mkfs.btrfs $backing_store
fi

if [ -z $(df $mount_point) ]
then
    echo "mounting btrfs volume"
    mkdir -p $mount_point
    mount -t btrfs $loopback_device $mount_point
else
    echo "btrfs volume already mounted"
fi
