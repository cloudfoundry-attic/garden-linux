#!/bin/bash

set -e -x

temp_dir_path=$(mktemp -d /tmp/build-tar-XXXXXX)
old_wd=$(pwd)
cd $temp_dir_path

apt-get update
apt-get install wget build-essential -y
wget http://ftp.gnu.org/gnu/tar/tar-1.28.tar.gz
tar zxf tar-1.28.tar.gz

cd tar-1.28
export LDFLAGS="-static"
export FORCE_UNSAFE_CONFIGURE=1
./configure

make
mv src/tar /opt/tar

cd $old_wd
rm -rf $temp_dir_path

