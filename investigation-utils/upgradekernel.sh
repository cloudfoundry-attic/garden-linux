#!/bin/bash

if [ -z $1 ] ; then
  echo "usage: $0 kernel-version";
fi

version=$1

apt-get install linux-image-$version linux-headers-$version linux-image-extra-$version -y
