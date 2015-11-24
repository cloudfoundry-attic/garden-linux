# Garden Linux

A Linux backend for [Garden](https://github.com/cloudfoundry-incubator/garden).

You can deploy Garden-Linux using the [Garden-Linux BOSH Release](https://github.com/cloudfoundry-incubator/garden-linux-release).

## Installing Garden-Linux

**Note:** the rest of these instructions assume you arranged for the garden-linux code and dependencies to be
present in your `$GOPATH` on a machine running Ubuntu 14.04 or later.

### Build garden-linux

```
cd $GOPATH/src/github.com/cloudfoundry-incubator/garden-linux # assuming your $GOPATH has only one entry
make
```

### Set up necessary directories

```
sudo mkdir -p /opt/garden/containers
sudo mkdir -p /opt/garden/snapshots
sudo mkdir -p /opt/garden/rootfs
sudo mkdir -p /opt/garden/graph
```

### Download a RootFS (Optional)

If you plan to run docker images instead of using rootfs from disk, you can skip this step.

e.g. if you want to use the default Cloud Foundry rootfs:
```
wget https://github.com/cloudfoundry/stacks/releases/download/1.19.0/cflinuxfs2-1.19.0.tar.gz
sudo tar -xzpf cflinuxfs2-1.19.0.tar.gz -C /opt/garden/rootfs
```

### Run garden-linux

```
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
```

### Kick the tyres

The easiest way to start creating containers is using the unofficial [`gaol`](https://github.com/contraband/gaol) command line client.
For more advanced use cases, you'll want to use the (Garden)[https://github.com/cloudfoundry-incubator/garden] client package.

## Development

See the [Developer's Guide](docs/DEVELOPING.md) to get started.

Many integration tests are in another repository, [Garden Integration Tests](https://github.com/cloudfoundry-incubator/garden-integration-tests).
