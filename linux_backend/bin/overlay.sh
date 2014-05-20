#!/bin/bash

set -e

action=$1
container_path=$2
overlay_path=$2/overlay
rootfs_path=$2/rootfs
base_path=$3

function overlay_directory_in_rootfs() {
  # Skip if exists
  if [ ! -d $overlay_path/$1 ]
  then
    if [ -d $rootfs_path/$1 ]
    then
      cp -r $rootfs_path/$1 $overlay_path/
    else
      mkdir -p $overlay_path/$1
    fi
  fi

  mount -n --bind $overlay_path/$1 $rootfs_path/$1
  mount -n --bind -o remount,$2 $overlay_path/$1 $rootfs_path/$1
}

function setup_fs_other() {
  mkdir -p $base_path/proc

  mount -n --bind $base_path $rootfs_path
  mount -n --bind -o remount,ro $base_path $rootfs_path

  overlay_directory_in_rootfs /dev rw
  overlay_directory_in_rootfs /etc rw
  overlay_directory_in_rootfs /home rw
  overlay_directory_in_rootfs /sbin rw
  overlay_directory_in_rootfs /var rw

  mkdir -p $overlay_path/tmp
  chmod 777 $overlay_path/tmp
  overlay_directory_in_rootfs /tmp rw
}

function get_mountpoint() {
  df -P $1 | tail -1 | awk '{print $NF}'
}

function current_fs() {
  mountpoint=$(get_mountpoint $1)

  local mp
  local fs

  while read _ mp fs _; do
    if [ "$fs" = "rootfs" ]; then
      continue
    fi

    if [ "$mp" = "$mountpoint" ]; then
      echo $fs
    fi
  done < /proc/mounts
}

function should_use_overlayfs() {
  # load it so it's in /proc/filesystems
  modprobe -q overlayfs >/dev/null 2>&1 || true

  # cannot mount overlayfs in aufs
  if [ "$(current_fs $overlay_path)" == "aufs" ]; then
    return 1
  fi

  # cannot mount overlayfs in overlayfs; whiteout not supported
  if [ "$(current_fs $overlay_path)" == "overlayfs" ]; then
    return 1
  fi

  # check if it's a known filesystem
  grep -q overlayfs /proc/filesystems
}

function should_use_aufs() {
  # load it so it's in /proc/filesystems
  modprobe -q aufs >/dev/null 2>&1 || true

  # cannot mount aufs in aufs
  if [ "$(current_fs $overlay_path)" == "aufs" ]; then
    return 1
  fi

  # cannot mount aufs in overlayfs
  if [ "$(current_fs $overlay_path)" == "overlayfs" ]; then
    return 1
  fi

  # check if it's a known filesystem
  grep -q aufs /proc/filesystems
}

function setup_fs() {
  mkdir -p $overlay_path
  mkdir -p $rootfs_path

  if should_use_aufs; then
    mount -n -t aufs -o br:$overlay_path=rw:$base_path=ro+wh none $rootfs_path
  elif should_use_overlayfs; then
    mount -n -t overlayfs -o rw,upperdir=$overlay_path,lowerdir=$base_path none $rootfs_path
  else
    setup_fs_other
  fi
}

function rootfs_mountpoints() {
  cat /proc/mounts | grep $rootfs_path | awk '{print $2}'
}

function teardown_fs() {
  umount $(rootfs_mountpoints) 2>/dev/null || true

  leftover=$(rootfs_mountpoints)
  if [ -n "$leftover" ]; then
    umount $leftover
  fi

  rm -rf $container_path
}

if [ "$action" = "create" ]; then
  setup_fs
else
  teardown_fs
fi
