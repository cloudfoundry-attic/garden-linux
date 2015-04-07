#!/bin/bash

set -e -x
apt-get update && apt-get -y install \
  fuse \
  libfuse-dev \
  pkg-config

cd /usr/share/doc/libfuse-dev/examples && \
  bash -c "gcc -Wall hello.c $(pkg-config fuse --cflags --libs) -o /usr/bin/hellofs"
