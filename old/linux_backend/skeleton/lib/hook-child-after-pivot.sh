#!/bin/sh

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit

cd $(dirname $0)/../

. etc/config

hostname $id

mkdir -p /proc
mount -n -t proc none /proc
mount -n -t tmpfs -o nodev tmpfs /dev/shm

if [ -e /etc/seed ]; then
  . /etc/seed
fi