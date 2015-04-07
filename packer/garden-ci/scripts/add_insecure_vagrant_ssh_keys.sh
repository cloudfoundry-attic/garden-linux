#!/bin/bash

set -e -x

apt-get install -y wget
ssh_dir=/home/vagrant/.ssh

mkdir -p $ssh_dir
chmod 0700 $ssh_dir

wget 'https://raw.github.com/mitchellh/vagrant/master/keys/vagrant.pub' -O $ssh_dir/id_rsa.pub
chmod 0644 $ssh_dir/id_rsa.pub

wget 'https://raw.github.com/mitchellh/vagrant/master/keys/vagrant' -O $ssh_dir/id_rsa
chmod 0600 $ssh_dir/id_rsa

cp $ssh_dir/{id_rsa.pub,authorized_keys}
chmod 0600 $ssh_dir/authorized_keys

chown -R vagrant:vagrant $ssh_dir
