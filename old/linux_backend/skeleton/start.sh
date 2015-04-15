#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)

source ./etc/config

if [ -f ./run/wshd.pid ]
then
  echo "wshd is already running..."
  exit 1
fi

./net.sh setup

if [ "$root_uid" -eq 0 ]
then
  ./bin/wshd --socket ./run/wshd.sock --lib ./lib --root $rootfs_path --title "wshd: $id" --userns disabled
else
  ./bin/wshd --socket ./run/wshd.sock --lib ./lib --root $rootfs_path --title "wshd: $id" --userns enabled
fi
