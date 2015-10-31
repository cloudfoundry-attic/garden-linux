#!/usr/bin/env bash

set -e

echo -n "losetup: " && losetup -a | wc -l
echo -n "diff: " && ls /var/vcap/data/garden/aufs_graph/aufs/diff | wc -l
echo -n "mnt: " && ls /var/vcap/data/garden/aufs_graph/aufs/mnt | wc -l
echo -n "depot: " && ls /var/vcap/data/garden/depot | wc -l
echo -n "mounts: " && grep loop /proc/mounts | wc -l
echo -n "backing stores: " && ls /var/vcap/data/garden/aufs_graph/backing_stores/* | wc -l
echo -n "unknown handles: " && grep 'unknown handle' /var/vcap/sys/log/garden/garden.stdout.log  | wc -l
echo "disk usage: "
du -h /var/vcap/data/garden/aufs_graph/backing_stores/* | cut -f 1 | sort | uniq -c
echo "Volumes"
df -h /var/vcap/data
