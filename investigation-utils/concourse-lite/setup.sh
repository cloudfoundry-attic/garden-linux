#!/bin/bash

vagrant destroy -f
pkill -9 fly
vagrant up
vagrant ssh -c 'sudo /vagrant/install-rootfs.sh'
vagrant reload 
