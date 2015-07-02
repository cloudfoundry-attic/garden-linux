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
    touch $backing_store
    truncate -s 3000M $backing_store
    loopback_device=$(losetup -f --show $backing_store)
    mkfs.btrfs --nodiscard $loopback_device
fi

if cat /proc/mounts | grep $mount_point
then
    echo "btrfs volume already mounted"
else
    echo "mounting btrfs volume"
    mkdir -p $mount_point
    mount -t btrfs $loopback_device $mount_point
fi
