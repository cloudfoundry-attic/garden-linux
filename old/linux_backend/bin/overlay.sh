#!/bin/bash

set -e

action=$1
container_path=$2
overlay_path=$2/overlay
workdir_path=$2/workdir
rootfs_path=$2/rootfs
base_path=$3

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

function should_use_overlay() {
  # load it so it's in /proc/filesystems
  modprobe -q overlay >/dev/null 2>&1 || true

  # assumption: cannot mount overlay in aufs
  if [ "$(current_fs $overlay_path)" == "aufs" ]; then
    return 1
  fi

  # assumption: cannot mount overlay in overlayfs
  if [ "$(current_fs $overlay_path)" == "overlayfs" ]; then
    return 1
  fi

  # assumption: cannot mount overlay in overlay
  if [ "$(current_fs $overlay_path)" == "overlay" ]; then
    return 1
  fi

  # check if it's a known filesystem
  grep -q overlay /proc/filesystems
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
  mkdir -p $workdir_path
  mkdir -p $rootfs_path

  if should_use_aufs; then
    mount -n -t aufs -o br:$overlay_path=rw:$base_path=ro+wh none $rootfs_path
  elif should_use_overlayfs; then
    mount -n -t overlayfs -o rw,upperdir=$overlay_path,lowerdir=$base_path none $rootfs_path
  elif should_use_overlay; then
    mount -n -t overlay -o rw,upperdir=$overlay_path,lowerdir=$base_path,workdir=$workdir_path none $rootfs_path
  else
    # aufs and overlayfs are the only supported mount types.
    # aufs and overlayfs can be used in nested containers by mounting
    # the overlay directories on e.g. tmpfs
    echo "the directories that contain the depot and rootfs must be mounted on a filesystem type that supports aufs or overlayfs" >&2
    exit 222
  fi
}

function rootfs_mountpoints() {
  cat /proc/mounts | grep $rootfs_path | awk '{print $2}'
}

function teardown_fs() {
  for i in $(seq 480); do
    local mountpoints=$(rootfs_mountpoints)
    if [ -z "$mountpoints" ] || umount $mountpoints; then
      if rm -rf $container_path; then
        return 0
      fi
    fi

    sleep 0.5
  done

  return 1
}

if [ "$action" = "create" ]; then
  setup_fs
else
  teardown_fs
fi
