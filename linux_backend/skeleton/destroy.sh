#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)

source ./etc/config

./net.sh teardown

cgroup_path="${WARDEN_CGROUP_PATH}"

if [ -f ./run/wshd.pid ]
then
  pid=$(cat ./run/wshd.pid)
  path=${cgroup_path}/cpu/instance-$id
  tasks=$path/tasks

  if [ -d $path ]
  then
    kill -9 $pid 2> /dev/null || true

    # Wait while there are tasks in one of the instance's cgroups
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
      # Remove nested cgroups for nested-warden
      rmdir $path/instance* 2> /dev/null || true
      rmdir $path
    fi
  done

  exit 0
fi
