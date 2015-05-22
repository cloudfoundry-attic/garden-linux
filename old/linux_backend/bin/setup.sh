#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname "${0}")

cgroup_path="${GARDEN_CGROUP_PATH}"

function mount_flat_cgroup() {
  cgroup_parent_path=$(dirname $1)

  mkdir -p $cgroup_parent_path

  if ! mountpoint -q $cgroup_parent_path; then
    mount -t tmpfs none $cgroup_parent_path
  fi

  mkdir -p $1
  mount -t cgroup cgroup $1

  # bind-mount cgroup subsystems to make file tree consistent
  for subsystem in $(tail -n +2 /proc/cgroups | awk '{print $1}'); do
    mkdir -p ${1}/$subsystem

    if ! mountpoint -q ${1}/$subsystem; then
      mount --bind $1 ${1}/$subsystem
    fi
  done
}

function mount_nested_cgroup() {
  mkdir -p $1

  if ! mountpoint -q $1; then
    mount -t tmpfs -o uid=0,gid=0,mode=0755 cgroup $1
  fi

  for subsystem in $(tail -n +2 /proc/cgroups | awk '{print $1}'); do
    mkdir -p ${1}/$subsystem

    if ! mountpoint -q ${1}/$subsystem; then
      mount -n -t cgroup -o $subsystem cgroup ${1}/$subsystem
    fi
  done
}

if [ ! -d $cgroup_path ]
then
  mount_nested_cgroup $cgroup_path || \
    mount_flat_cgroup $cgroup_path
fi

./net.sh setup

# Disable AppArmor if possible
if [ -x /etc/init.d/apparmor ]; then
  /etc/init.d/apparmor teardown
fi

# disable quotaon-style quotas
