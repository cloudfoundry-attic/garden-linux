#Building a new Garden Linux Vagrant Box
1. Follow [instructions](https://github.com/CpuID/packer-ubuntu-virtualbox) to build
a compatable Ubuntu VirtualBox base vmdk / ovf, due to [this issue](https://github.com/mitchellh/packer/issues/1726)
1. Run `./setup`
1. Run `vagrant box add garden-linux --force`
1. Run `vagrant init garden-linux`
1. Run `vagrant up`
1. You can now ssh in via `vagrant ssh`

##What is happening?

We are exporting a flattened docker image and then extracting it and creating a new vagrant box. 

Note upon `vagrant up` your go path with be sync'd to ~/warden/go. We suggest that you chroot into ~/warden to get a similar environment to running Garden via concourse or in docker directly.

#Using an existing Garden Linux Vagrant Box

You can find a pre-built box here: https://atlas.hashicorp.com/avade/boxes/garden-linux-test

This may not be the latest but should get you up and running.