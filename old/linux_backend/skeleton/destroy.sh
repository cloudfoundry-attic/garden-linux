#!/bin/bash

set -x

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)

source ./etc/config

./net.sh teardown

cgroup_path="${GARDEN_CGROUP_PATH}"

if [ -f ./run/wshd.pid ]
then
  pid=$(cat ./run/wshd.pid)

  # Arbitrarily pick the cpu substem to check for live tasks.
  path=${cgroup_path}/cpu/instance-$id
  tasks=$path/tasks

  if [ -d $path ]
  then
    # Kill the container's init pid; the kernel will reap all tasks.
    kill -9 $pid

    # Wait while there are tasks in one of the instance's cgroups.
    #
    # Even though we've technically killed the root of the pid namespace,
    # it can take a brief period of time for the kernel to reap.
    while [ -f $tasks ] && [ -n "$(cat $tasks)" ]; do
      sleep 0.1
    done
  fi

  # Done, remove pid
  rm -f ./run/wshd.pid

  # Remove cgroups
  for system_path in ${cgroup_path}/*
  do
    path=$system_path/instance-$id

    if [ -d $path ]
    then
      # Recursively remove all cgroup trees under (and including) the instance.
      #
      # Running another containerization tool in the container may create these,
      # and the parent cannot be removed until they're removed first.
      #
      # find .. -delete ensures that it processes them depth-first.
      find $path -type d -delete
    fi
  done
fi

# The kernel takes a bit to reap the container's mount namespace, which may
# contain mountpoints under these (shared) directories.
#
# In manual testing, retrying appears to be a reasonable solution.
function rmdir_with_retry() {
  until rmdir "$@"; do
    sleep 0.1
  done
}

# all umounts below appear to be flaky; they sometimes "disappear" from the
# mount table, seemingly as other things bind-mount to them. for example, with
# N containers bind-mounting to the same volume, sometimes only the last
# container bound to it will actually have these mounts present.

# Clean up shared volumes
for volume in ${PWD}/volumes/*; do
  umount $volume || true
  rmdir_with_retry $volume
done

for binding in ${PWD}/bindings/*; do
  umount $binding || true
  rmdir_with_retry $binding
done

umount volumes || true
rmdir_with_retry volumes
