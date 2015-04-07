#!/bin/bash
set -e -x

# install build dependencies

apt-get -y install \
  dkms \
  linux-headers-$(uname -r) 
