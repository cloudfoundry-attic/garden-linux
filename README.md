# Garden Linux

A Linux backend for [Garden](https://github.com/cloudfoundry-incubator/garden).

You can deploy Garden (inside a Garden container) using the [Garden BOSH Release repository](https://github.com/cloudfoundry-incubator/garden-linux-release).

See the [old README](old/README.md) for old documentation, caveat lector.

## Installing Garden-Linux

**Note:** the rest of these instructions assume you arranged for the garden-linux code, and dependencies, to be
installed in your `$GOPATH` inside a linux environment, either by following the steps above or through some other mechanism.

The rest of these instructions assume you are running inside an Ubuntu environment (for example, the above vagrant box) with go installed and the code checked out.

* Install Btrfs tools if not present on your machine

        apt-get update
        apt-get install -y asciidoc xmlto --no-install-recommends
        apt-get install -y pkg-config autoconf
        apt-get build-dep -y btrfs-tools

        mkdir -p /tmp/btrfs
        cd /tmp/btrfs
        git clone git://git.kernel.org/pub/scm/linux/kernel/git/kdave/btrfs-progs.git
        cd btrfs-progs
        ./autogen.sh
        ./configure
        make && make install

* Setup a loopback device as a btrfs formatted volume. The commands below should be run as root

        backing_store=/opt/garden/btrfs_backing_store
        loopback_device=/dev/btrfs_loop
        mount_point=/opt/garden/graph

        if [ ! -d $mount_point ]
        then
            dd if=/dev/zero of=$backing_store bs=1M count=3000  # Here we are writing 3GB. You can adjust this value accordingly.
            mknod $loopback_device b 7 200
            losetup $loopback_device $backing_store
            mkfs.btrfs $backing_store
        fi

        if cat /proc/mounts | grep $mount_point
        then
            echo "btrfs volume already mounted"
        else
            echo "mounting btrfs volume"
            mkdir -p $mount_point
            mount -t btrfs $loopback_device $mount_point
        fi

* Build garden-linux

        cd $GOPATH/src/github.com/cloudfoundry-incubator/garden-linux # assuming your $GOPATH has only one entry
        make
        go build -a -tags daemon -o out/garden-linux

* Set up necessary directories

        sudo mkdir -p /opt/garden/containers
        sudo mkdir -p /opt/garden/snapshots
        sudo mkdir -p /opt/garden/rootfs
        sudo mkdir -p /opt/garden/graph

* (Optional) Set up a RootFS

    If you plan to run docker images instead of using the warden rootfs provider, you can skip this step.

    Follow the instructions at [https://github.com/cloudfoundry/stacks](https://github.com/cloudfoundry/stacks) to generate a rootfs, or download one from `http://cf-runtime-stacks.s3.amazonaws.com/lucid64.dev.tgz`. Extract it to `/opt/warden/rootfs` (or pass a different directory in the next step).

        wget http://cf-runtime-stacks.s3.amazonaws.com/lucid64.dev.tgz
        sudo tar -xzpf lucid64.dev.tgz -C /opt/garden/rootfs

* Run garden-linux

        cd $GOPATH/src/github.com/cloudfoundry-incubator/garden-linux # assuming your $GOPATH has only one entry
        sudo ./out/garden-linux \
               -depot=/opt/garden/containers \
               -bin=$PWD/linux_backend/bin \
               -rootfs=/opt/garden/rootfs \
               -graph=/opt/garden/graph \
               -snapshots=/opt/garden/snapshots \
               -listenNetwork=tcp \
               -listenAddr=127.0.0.1:7777 \
               "$@"

* Kick the tyres

    The external API is exposed using [Garden](https://github.com/cloudfoundry-incubator/garden), the instructions at that repo document the various API calls that you can now make (it will be running at `http://127.0.0.1:7777` if you followed the above instructions).

## External API

The `garden-linux` executable provides a server which clients can use to perform operations on Garden Linux,
such as creating containers and running processes inside containers.
    
Garden Linux is configured by passing command line flags to the `garden-linux` executable.

[Garden](https://github.com/cloudfoundry-incubator/garden) defines the protocol supported by the server and provides a Go API for programmatic access.

## Development

See the [Developer's Guide](docs/DEVELOPING.md) to get started.

Many integration tests are in another repository, [Garden Integration Tests](https://github.com/cloudfoundry-incubator/garden-integration-tests).
