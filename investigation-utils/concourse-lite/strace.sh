#!/bin/bash
# set -x

if [ $# -ne 1 ]; then
  echo "USAGE: $0 <Binary you want to trace>"
  exit 1
fi
binary=$1

arr=(`pidof $binary`)
args=""
for pid in ${arr[@]}; do
  args="$args -p $pid"
done
strace $args
