# Garden-Linux Packer #

Garden-Linux Packer is currently used to build Docker images / Vagrant boxes
suitable for Garden-Linux development & testing.

## Pre-requisites for Mac

* Boot2docker version
  [1.3.3](https://github.com/boot2docker/osx-installer/releases/tag/v1.3.3) (1.4
  and above does not work because of [this
  issue](https://github.com/mitchellh/packer/issues/1752))
* Packer built from [source](https://github.com/mitchellh/packer) (latest
  master)

## Building

For some reason the default temp dir for packer has issues with the boot2docker
vm. In order to build images you will need to export the `TMPDIR`. For
instance, if you are using [direnv](http://direnv.net/) add this to a `.envrc`
in the packer directory:

```bash
export TMPDIR=~/.packer_tmp
mkdir -p $TMPDIR
```

###Build everything

Run `make ubuntu`. This will output a virtual-box `.ovf` and vagrant `.box` to
`garden-ci/output` and commit a docker image to your Docker server named
`garden-ci-ubuntu:packer`.

### Build Individual Images
  * Docker: `make ubuntu-docker`
  * Vagrant: `make ubuntu-vagrant`
  * Amazon: `make ubuntu-ami`

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

### [Amazon EC2](http://aws.amazon.com/ec2/)

Since the ami exists on Amazon, there is no need to release it. We just need to
record its Image Id so that builds will use it and make the ami public.

Store the Image id of the ami in the file `garden-ci/AMI_IMAGE_ID` and commit
and push.

Make the ami public (see [Making an AMI Public](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/sharingamis-intro.html):
```bash
aws ec2 describe-image-attribute --image-id <Image Id> --attribute launchPermission
```

TODO

