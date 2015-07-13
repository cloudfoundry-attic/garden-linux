# Garden-Linux Packer

Garden-Linux Packer is currently used to build Docker images / Vagrant boxes
suitable for Garden-Linux development & testing.

## Pre-requisites for Mac

* Boot2docker version
  [1.3.3](https://github.com/boot2docker/osx-installer/releases/tag/v1.3.3) (1.4
  and above does not work because of [#1752](https://github.com/mitchellh/packer/issues/1752))
* Packer version v0.8.0 from homebrew (`brew install packer`) or [the site](https://www.packer.io/downloads.html)

## Building

For some reason the default temp dir for packer has issues with the boot2docker
vm. In order to build images you will need to export the `TMPDIR`. For
instance, if you are using [direnv](http://direnv.net/) add this to a `.envrc`
in the packer directory:

```bash
export TMPDIR=~/.packer_tmp
mkdir -p $TMPDIR
```

### Build everything

Run `make ubuntu`. This will output a virtual-box `.ovf` and vagrant `.box` to
`garden-ci/output` and commit a docker image to your Docker server named
`garden-ci-ubuntu:packer`.

### Build Individual Images

  * Docker: `make ubuntu-docker`
  * Vagrant: `make ubuntu-vagrant`

## Releasing

Update `garden-ci/version.json` with the desired version number.

### [Atlas](https://atlas.hashicorp.com/)

Ensure that you have the correct environment varibles set.

```bash
export GARDEN_PACKER_ATLAS_TOKEN=<Token goes here>
```

Then run `make release-vagrant`. This will build & upload vagrant box upto Atlas.

### [DockerHub](https://hub.docker.com/)

Ensure that you have the correct environment varibles set.

```bash
export GARDEN_PACKER_DOCKER_USERNAME=<Docker user name>
export GARDEN_PACKER_DOCKER_EMAIL=<Docker email>
export GARDEN_PACKER_DOCKER_PASSWORD=<Docker password>
```

Then run `make release-docker`. This will tag and upload the image to Docker Hub.