#!/bin/bash

set -e -x

if [ -z `getent group admin` ]; then
  groupadd -r admin
fi
usermod -a -G admin root

sed -i -e '/Defaults\s\+env_reset/a Defaults\texempt_group=admin' /etc/sudoers
sed -i -e 's/%admin ALL=(ALL) ALL/%admin ALL=NOPASSWD:ALL/g' /etc/sudoers
