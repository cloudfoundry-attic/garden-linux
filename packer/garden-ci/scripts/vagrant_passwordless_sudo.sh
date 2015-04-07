#!/bin/bash

set -e -x

usermod -a -G admin vagrant
echo 'vagrant ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers
