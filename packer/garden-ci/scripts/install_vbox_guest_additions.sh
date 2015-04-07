#!/bin/bash

set -e -x

## install the vbox guest additions
mkdir -p /mnt/VBoxGuestAdditions
mount /home/vagrant/VBoxGuestAdditions.iso /mnt/VBoxGuestAdditions

set +e
#TODO: this runs but reports a failure due to x11. Find a better way to handle this
/mnt/VBoxGuestAdditions/VBoxLinuxAdditions.run
set -e
 
## cleanup the vbox guest additions
umount /mnt/VBoxGuestAdditions
rmdir /mnt/VBoxGuestAdditions
rm /home/vagrant/VBoxGuestAdditions.iso
