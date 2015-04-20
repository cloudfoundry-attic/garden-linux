#!/bin/bash

set -e -x

usermod -a -G admin ubuntu
echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers
