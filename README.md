# Garden Linux

A Linux backend for [Garden](https://github.com/cloudfoundry-incubator/garden).

You can deploy Garden using the [Garden BOSH Release repository](https://github.com/cloudfoundry-incubator/garden-linux-release).

See the [old README](old/README.md) for old documentation, caveat lector.

## Creating a suitable dev box to install the code

A good vagrant box to start from is at [cf-guardian/dev](http://github.com/cf-guardian/dev). 

If you downloaded the dev box above, then you can set up the dependencies in your host machine (as follows), and then - making sure `$GOHOME` is properly set, say `vagrant up` in the cloned [cf-guardian/dev](http://github.com/cf-guardian/dev) box to create a vagrant with the source code checked out.

First, follow the [godep instructions](http://github.com/tools/godep) to install godep.

Then, checkout the code and restore the dependencies with godeps (this assumes your `$GOPATH` is a single value):

    git clone https://github.com/cloudfoundry-incubator/garden-linux $GOPATH/src/github.com/cloudfoundry-incubator/garden-linux
    cd $GOPATH/src/github.com/cloudfoundry-incubator/garden-linux
    godep restore

Now, make sure to set `$GOHOME` to `$GOPATH` so that cf-guardian dev box knows where to find your go code:

    export GOHOME=$GOPATH # assuming your GOPATH only contains one entry

Bring dev box up:

    cd ~/workspace/dev # or wherever you checked out the dev box
    vagrant up
    vagrant ssh


## Installing Garden-Linux

**Note:** the rest of these instructions assume you arranged for the garden-linux code, and dependencies, to be
installed in your `$GOPATH` inside a linux environment, either by following the steps above or through some other mechanism.

The rest of these instructions assume you are running inside an Ubuntu environment (for example, the above vagrant box) with go installed and the code checked out.

* Build garden-linux

        cd $GOPATH/src/github.com/cloudfoundry-incubator/garden-linux # assuming your $GOPATH has only one entry
        make
        go build -a -tags daemon -o out/garden-linux

* Set up necessary directories

        sudo mkdir -p /opt/garden/containers
        sudo mkdir -p /opt/garden/snapshots
        sudo mkdir -p /opt/garden/overlays
        sudo mkdir -p /opt/garden/rootfs

* (Optional) Set up a RootFS

    If you plan to run docker images instead of using the warden rootfs provider, you can skip this step.

    Follow the instructions at [https://github.com/cloudfoundry/stacks](https://github.com/cloudfoundry/stacks) to generate a rootfs, or download one from `http://cf-runtime-stacks.s3.amazonaws.com/lucid64.dev.tgz`. Extract it to `/opt/warden/rootfs` (or pass a different directory in the next step).

        wget http://cf-runtime-stacks.s3.amazonaws.com/lucid64.dev.tgz
        sudo tar -xzpf lucid64.dev.tgz -C /opt/garden/rootfs

* Run garden-linux

        cd $GOPATH/src/github.com/cloudfoundry-incubator/garden-linux # assuming your $GOPATH has only one entry
        sudo ./out/garden-linux \
               -depot=/opt/garden/containers \
               -bin=$PWD/old/linux_backend/bin \
               -rootfs=/opt/garden/rootfs \
               -snapshots=/opt/garden/snapshots \
               -overlays=/opt/garden/overlays \
               -listenNetwork=tcp \
               -listenAddr=127.0.0.1:7777 \
               "$@"

* Kick the tyres

    The external API is exposed using [Garden](https://github.com/cloudfoundry-incubator/garden), the instructions at that repo document the various API calls that you can now make (it will be running at `http://127.0.0.1:7777` if you followed the above instructions).

## Using the supplied Vagrantfile to install Garden-linux inside vagrant

Follow the steps below to create a vagrant box with garden-linux installed.

```bash
# if you need it:
vagrant plugin install vagrant-omnibus

# then:
librarian-chef install
vagrant up
```

With the box configured as above, you can run `./scripts/test-in-vagrant` (on your local machine) to run the test suite.

## External API

The `garden-linux` executable provides a server which clients can use to perform operations on Garden Linux,
such as creating containers and running processes inside containers.
    
Garden Linux is configured by passing command line flags to the `garden-linux` executable.

[Garden](https://github.com/cloudfoundry-incubator/garden) defines the protocol supported by the server and provides a Go API for programmatic access.

## Development

Restructure in progress: code in the `old/` directory is being replaced with code elsewhere in the repository.

See the [Developer's Guide](docs/DEVELOPING.md) to get started.
