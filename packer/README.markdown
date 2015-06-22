# Garden-Linux Packer

Garden-Linux Packer is currently used to build Docker images / Vagrant boxes
suitable for Garden-Linux development & testing.

## Pre-requisites for Mac

* Boot2docker version
  [1.3.3](https://github.com/boot2docker/osx-installer/releases/tag/v1.3.3) (1.4
  and above does not work because of [#1752](https://github.com/mitchellh/packer/issues/1752))
* Packer version v0.7.5 from homebrew (`brew install packer`)

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
  * Amazon: `make ubuntu-ami`

## Releasing

Update `garden-ci/version.json` with the desired version number.

### [Atlas](https://atlas.hashicorp.com/)

**NOTE:** Because of the issue [#2090](https://github.com/mitchellh/packer/issues/2090), we cannot use the version number from `garden-ci/version.json`. The issue is fixed but no new stable version of Packer has been released since then. Until further notice (Packer upgrade), you need to update `garden-ci/release_vagrant.json` metadata section with the desired version number as well.

Ensure that you have the correct environment varibles set.

```bash
export GARDEN_PACKER_ATLAS_TOKEN=<Token goes here>
```

Then run `make release-vagrant`. This will build & upload vagrant box upto Atlas.

### [DockerHub](https://hub.docker.com/)

**NOTE:** Before running `release-docker` you need to retag the old `garden-ci-ubuntu` Docker image and remove the `latest` tag.

```bash
docker tag cloudfoundry/garden-ci-ubuntu:latest cloudfoundry/garden-ci-ubuntu:0.4.0 # last version
docker rmi cloudfoundry/garden-ci-ubuntu:latest
```

This is temporary. According to the resolution of the issue [#1923](https://github.com/mitchellh/packer/issues/1923) Packer has now a `forced` flag that can be used to force `docker-tag` post-processing tasks regardless of the machine's current tags. This will become available to us once Packer releases the next stable version.

Ensure that you have the correct environment varibles set.

```bash
export GARDEN_PACKER_DOCKER_USERNAME=<Docker user name>
export GARDEN_PACKER_DOCKER_EMAIL=<Docker email>
export GARDEN_PACKER_DOCKER_PASSWORD=<Docker password>
```

Then run `make release-docker`. This will tag and upload the image to Docker Hub.

### [Amazon EC2](http://aws.amazon.com/ec2/)

Since the ami exists on Amazon, there is no need to release it. We just need to
record its Image Id so that builds will use it and make the ami public.

Store the Image id of the ami in the file `garden-ci/AMI_IMAGE_ID` and commit
and push.

Make the ami public (see [Making an AMI Public](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/sharingamis-intro.html)):

```bash
aws ec2 describe-image-attribute --image-id <Image Id> --attribute launchPermission
```
