#!/bin/sh

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit

cd $(dirname $0)/../

. etc/config

hostname $id

if [ -e /etc/seed ]; then
  . /etc/seed
fi
