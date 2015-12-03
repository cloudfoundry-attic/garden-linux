#!/bin/bash
set -x

cp /vagrant/garden-ci-ubuntu.tgz /var/vcap/data/garden/aufs_graph

cd /var/vcap/data/garden/aufs_graph
tar -zxf garden-ci-ubuntu.tgz

mv ./garden-ci-ubuntu/graph/* ./
mv ./garden-ci-ubuntu/diff/* ./aufs/diff
mv ./garden-ci-ubuntu/mnt/* ./aufs/mnt
mv ./garden-ci-ubuntu/layers/* ./aufs/layers

rm -Rf ./garden-ci-ubuntu
rm ./garden-ci-ubuntu.tgz

