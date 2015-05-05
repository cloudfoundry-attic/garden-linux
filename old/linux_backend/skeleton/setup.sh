#!/bin/bash

set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)

# Defaults for debugging the setup script
iface_name_prefix="${GARDEN_NETWORK_INTERFACE_PREFIX}"
max_id_len=$(expr 16 - ${#iface_name_prefix} - 2)
iface_name=$(tail -c ${max_id_len} <<< ${id})
id=${id:-test}
network_cidr=${network_cidr:-10.0.0.0/30}
container_iface_mtu=${container_iface_mtu:-1500}
network_host_ip=${network_host_ip:-10.0.0.1}
network_host_iface="${iface_name_prefix}${iface_name}-0"
network_container_ip=${network_container_ip:-10.0.0.2}
network_container_iface="${iface_name_prefix}${iface_name}-1"
bridge_iface="${bridge_iface}"
network_cidr_suffix=${network_cidr_suffix:-30}
user_uid=${user_uid:-10000}
root_uid=${root_uid:-10000}
rootfs_path=$(readlink -f $rootfs_path)

if [ ! -d $rootfs_path/tmp ]; then
  mkdir $rootfs_path/tmp
fi
chmod 1777 $rootfs_path/tmp

if [ ! -d $rootfs_path/etc ]; then
  mkdir $rootfs_path/etc
  chmod 0755 $rootfs_path/etc
fi

# Write configuration
cat > etc/config <<-EOS
id=$id
network_host_ip=$network_host_ip
network_host_iface=$network_host_iface
network_container_ip=$network_container_ip
network_container_iface=$network_container_iface
bridge_iface=$bridge_iface
network_cidr_suffix=$network_cidr_suffix
container_iface_mtu=$container_iface_mtu
network_cidr=$network_cidr
root_uid=$root_uid
user_uid=$user_uid
rootfs_path=$rootfs_path
external_ip=$external_ip
EOS

if [ ! -d $rootfs_path/proc ]; then
  mkdir -p $rootfs_path/proc
  chown $root_uid:$root_uid $rootfs_path/proc
  chmod 0755 $rootfs_path/proc
fi

#chown $root:0 $rootfs_path/proc

if [ ! -d $rootfs_path/dev ]; then
  mkdir -p $rootfs_path/dev
  chown $root_uid:$root_uid $rootfs_path/dev
  chmod 0755 $rootfs_path/dev
fi

# Strip /dev down to the bare minimum
rm -rf $rootfs_path/dev/*

if [ ! -d $rootfs_path/dev/shm ]; then
  mkdir $rootfs_path/dev/shm
  chown $root_uid:$root_uid $rootfs_path/dev/shm
  chmod 1777 $rootfs_path/dev/shm
fi

# add device: adddev <owner> <device-file-path> <mknod-1> <mknod-2>
function adddev()
{
  local own=${1}
  local file=${2}
  local opts="c ${3} ${4}"

  mknod -m 666 ${file} ${opts}
  chown root:${own} ${file}
}


# /dev/tty
adddev tty  $rootfs_path/dev/tty     5 0
# /dev/random, /dev/urandom
adddev root $rootfs_path/dev/random  1 8
adddev root $rootfs_path/dev/urandom 1 9
# /dev/null, /dev/zero, /dev/full
adddev root $rootfs_path/dev/null    1 3
adddev root $rootfs_path/dev/zero    1 5
adddev root $rootfs_path/dev/full    1 7

# /dev/fd, /dev/std{in,out,err}
pushd $rootfs_path/dev > /dev/null
ln -s /proc/self/fd
ln -s fd/0 stdin
ln -s fd/1 stdout
ln -s fd/2 stderr
popd > /dev/null

# Add fuse group and device, so fuse can work inside the container
mknod -m 666 $rootfs_path/dev/fuse c 10 229
chown $root_uid:$root_uid $rootfs_path/dev/fuse
chmod ugo+rw $rootfs_path/dev/fuse

cat > $rootfs_path/etc/hostname <<-EOS
$id
EOS

cat > $rootfs_path/etc/hosts <<-EOS
127.0.0.1 localhost
$network_container_ip $id
EOS

# By default, inherit the nameserver from the host container.
#
# Exception: When the host's nameserver is set to localhost (127.0.0.1), it is
# assumed to be running its own DNS server and listening on all interfaces.
# In this case, the container must use the network_host_ip address
# as the nameserver.
if [[ "$(cat /etc/resolv.conf)" == "nameserver 127.0.0.1" ]]
then
  cat > $rootfs_path/etc/resolv.conf <<-EOS
nameserver $network_host_ip
EOS
else
  # some images may have something set up here; the host's should be the source
  # of truth
  rm -f $rootfs_path/etc/resolv.conf

  cp /etc/resolv.conf $rootfs_path/etc/
fi


# Add vcap user if not already present
if ! chroot $rootfs_path id vcap >/dev/null 2>&1; then
  mkdir -p $rootfs_path/home

  shell=/bin/sh
  if [ -f $rootfs_path/bin/bash ]; then
    shell=/bin/bash
  fi

  touch $rootfs_path/etc/passwd
  touch $rootfs_path/etc/group
  useradd -R $rootfs_path -m -u 101000 -U -s $shell vcap
  vcap_uid=$(($root_uid + 101000))
  chown $vcap_uid:$vcap_uid $rootfs_path/home/vcap
fi

# workaround aufs limitations by copying /root directory out and back
# in order to get it a new inode. This is the only way to prevent the
# ownership in the read-only layer affecting the read-write layer. Later
# versions of aufs support a dirperm1 mount option which should allow us
# to remove this workaround.
rm -rf "$rootfs_path/tmp/root" || true # just in case
[ -d "$rootfs_path/root" ] && cp -r "$rootfs_path/root" "$rootfs_path/tmp/root"
if rm -r "$rootfs_path/root" 2>&1; then
  mv "$rootfs_path/tmp/root" "$rootfs_path/root"
fi
rm -rf "$rootfs_path/tmp/root"

# change the root user id in the rootfs /root dir to the container root uid if they
# differ and if /root exists
if [ -d "$rootfs_path/root" ] && [ "$root_uid" -ne 0 ]; then
  chown -R --from=0:0 $root_uid:$root_uid "$rootfs_path/root" || true # ignore failures
fi

exit 0
