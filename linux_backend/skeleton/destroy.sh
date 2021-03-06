#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)

source ./etc/config

cgroup_path="${GARDEN_CGROUP_PATH}"

if [ -f ./run/wshd.pid ]
then
  pid=$(cat ./run/wshd.pid)

  # Arbitrarily pick the cpu substem to check for live tasks.
  cgroup_path_segment=$(cat /proc/self/cgroup | grep cpu: | cut -d ':' -f 3)
  path=${cgroup_path}/cpu${cgroup_path_segment}/instance-$id
  tasks=$path/tasks

  if [ -d $path ]
  then
    # Kill the container's init pid; the kernel will reap all tasks.
    kill -9 $pid || echo "wshd process seems to be gone already..."

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
  for subsystem in {cpuset,cpu,cpuacct,devices,memory}
  do
    cgroup_path_segment=$(cat /proc/self/cgroup | grep ${subsystem}: | cut -d ':' -f 3)
    path=${cgroup_path}/${subsystem}${cgroup_path_segment}/instance-$id

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

  exit 0
fi

