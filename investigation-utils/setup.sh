#!/bin/bash

echo "Do we currently have aufs loaded?"
lsmod | grep aufs || true
echo "Let's make aufs load!"
modprobe aufs
echo "NOW do we have aufs loaded?"
lsmod | grep aufs

echo "Ok, let's set up the gopath'n'stuff"

export GOPATH=/vagrant
export PATH=/vagrant/bin:$PATH

echo "now the gopath is $GOPATH and the path is $PATH"

echo "finally, let's set up an aufs volume on /tmp/aufs. First, create /tmp/aufs, /tmp/ro, and /tmp/rw"
mkdir /tmp/aufs /tmp/ro /tmp/rw

echo "now do the mount"
mount -t aufs -o br:/tmp/rw:/tmp/ro MyAUFSMount /tmp/aufs

echo "that seems to be done. You should be able to see it here:"
mount | grep MyAUFSMount

echo "....and a tmpfs inside it"
mkdir /tmp/aufs/tmpfs

mount -t tmpfs MyTMPFSMount /tmp/aufs/tmpfs

echo "that seems to be done too!. You should be able to see it here:"
mount | grep MyTMPFSMount
