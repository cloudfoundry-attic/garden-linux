#!/bin/bash

set -e
set -x

function prepareBtrfs()
{
    apt-get update
    apt-get install -y asciidoc xmlto --no-install-recommends
    apt-get install -y pkg-config autoconf
    apt-get build-dep -y btrfs-tools
    mkdir -p $HOME/btrfs
    pushd $HOME/btrfs
        git clone git://git.kernel.org/pub/scm/linux/kernel/git/kdave/btrfs-progs.git
        cd btrfs-progs
        ./autogen.sh
        ./configure
        make
        make install
    popd
}

if [ $(whoami) != "root" ]
then
    echo "Must be run as root" >&2
    exit 1
fi

if [ ! -f /usr/include/btrfs/version.h ]
then
    prepareBtrfs
else
    echo "btrfs tools already installed"
fi

backing_store=/opt/btrfs_backing_store
loopback_device=/dev/btrfs_loop
mount_point=/tmp/btrfs_mount
if [ ! -f $backing_store ]
then
    dd if=/dev/zero of=$backing_store bs=1M count=3000
    mknod $loopback_device b 7 200
    losetup $loopback_device $backing_store
    mkfs.btrfs $backing_store
    mkdir -p $mount_point
    mount -t btrfs $loopback_device $mount_point
    btrfs quota enable $mount_point
else
    echo "btrfs mount already set up, skipping"
fi
