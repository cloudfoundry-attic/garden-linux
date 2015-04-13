# Garden-Linux Packer #

Garden-Linux Packer is currently used to build Docker images / Vagrant boxes suitable for Garden-Linux development & testing.

## Pre-requisites for Mac

* Boot2docker version [1.3.3](https://github.com/boot2docker/osx-installer/releases/tag/v1.3.3)
* Packer built from [source](https://github.com/mitchellh/packer) (latest master)

## Building

For some reason the default temp dir for packer has issues with the boot2docker vm. In order to build images you will need to export the `TMPDIR`. If you are using [direnv](http://direnv.net/) add this to your `.envrc`.

```bash
export TMPDIR=~/.packer_tmp
mkdir $TMPDIR
```

###Build everything

Run `make ubuntu`. This will output a virtual-box `.ovf` and vagrant `.box` to `garden-ci/output` and commit a docker image to your Docker server named `garden-ci-ubuntu:packer`.

### Build Individual Images
  * Docker: `make ubuntu-docker`
  * Vagrant: `make ubuntu-vagrant`

## Releasing

### [Atlas](https://atlas.hashicorp.com/)

Update `garden-ci/VAGRANT_VIRTUAL_BOX_VERSION` with the desired version number.

Ensure that you have the correct environment varibles set.

```bash
export GARDEN_PACKER_VAGRANT_BOX_TAG=<Box tag goes here> # optional, defaults to cloudfoundry/garden-ci-ubuntu
export GARDEN_PACKER_ATLAS_TOKEN=<Token goes here>
```

Then run `make release-vagrant`. This will build & upload vagrant box upto Atlas.

### [DockerHub](https://hub.docker.com/)

Update `garden-ci/DOCKER_IMAGE_VERSION` with the desired version number.

Ensure that you have the correct environment varibles set.

```bash
export GARDEN_PACKER_DOCKER_USERNAME=<Docker user name>
export GARDEN_PACKER_DOCKER_EMAIL=<Docker email>
export GARDEN_PACKER_DOCKER_PASSWORD=<Docker password>
export GARDEN_PACKER_DOCKER_REPO=<Docker repo to target> # optional, defaults to cloudfoundry/garden-ci-ubuntu
```

Then run `make release-docker`. This will tag and upload the image to Docker Hub.
