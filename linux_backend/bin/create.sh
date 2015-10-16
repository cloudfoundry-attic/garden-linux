#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname "${0}")/..

if [ $# -ne 1 ]; then
  echo "Usage: ${0} <instance_path>"
  exit 1
fi

target=${1}

if [ ! -d ${target} ]; then
  echo "\"${target}\" does not exist, aborting..."
  exit 1
fi

cp -r skeleton/* "${target}"/.
ln -s `pwd`/bin/wshd "${target}"/bin/
ln -s `pwd`/bin/iodaemon "${target}"/bin/
ln -s `pwd`/bin/wsh "${target}"/bin/
ln -s `pwd`/bin/nstar "${target}"/bin/

unshare -m "${target}"/setup.sh
echo ${target}
