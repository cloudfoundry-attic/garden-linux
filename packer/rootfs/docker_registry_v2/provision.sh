#!/bin/bash

set -ex

docker pull busybox
docker tag -f busybox localhost:5000/busybox
docker push localhost:5000/busybox
