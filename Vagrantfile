# vim: set ft=ruby

Vagrant.configure("2") do |config|
  config.vm.hostname = "garden"

  config.vm.box = "trusty64"
  config.vm.box_url = "https://cloud-images.ubuntu.com/vagrant/trusty/current/trusty-server-cloudimg-amd64-vagrant-disk1.box"

  config.omnibus.chef_version = :latest

  config.vm.network "private_network", ip: "192.168.50.5"

  20.times do |i|
    config.vm.network "forwarded_port", guest: 7012 + i, host: 7012 + i
  end

  config.vm.synced_folder "/", "/host"
  config.vm.synced_folder ENV["GOPATH"], "/go"

  config.vm.provider :virtualbox do |v, override|
    v.customize ["modifyvm", :id, "--memory", 8*1024]
    v.customize ["modifyvm", :id, "--cpus", 4]
  end

  config.vm.provision :chef_solo do |chef|
    chef.cookbooks_path = ["cookbooks", "site-cookbooks"]

    chef.add_recipe "garden::apt-update"
    chef.add_recipe "build-essential::default"
    chef.add_recipe "golang"
    chef.add_recipe "garden::garden"
    chef.add_recipe "garden::rootfs"
    chef.add_recipe "garden::dev"

    chef.json = {
      go: {
        version: "1.3",
      },
    }
  end
end
