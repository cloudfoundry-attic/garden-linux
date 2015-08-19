ROOTFS_TARS = rootfs/busybox/busybox.tar \
	rootfs/docker_registry/docker_registry.tar \
	rootfs/docker_registry_v2/docker_registry_v2.tar \
	rootfs/fuse/fuse.tar \
	rootfs/preexisting_users/preexisting_users.tar \
	rootfs/ubuntu/ubuntu.tar
GARDEN_CI_DEPS = garden-ci/garden-ci.json \
	garden-ci/http/preseed.cfg

.PHONY: all clean ubuntu ubuntu-docker ubuntu-vagrant release-docker release-vagrant

all: ubuntu

clean:
	rm -fr build
	find rootfs -name '*.tar' -exec rm {} \;
	find . -type d -name 'output' -exec rm -rf {} \;

#
# Rootfs
#

rootfs/busybox/busybox.tar: rootfs/busybox/busybox.json rootfs/busybox/seed
	packer build rootfs/busybox/busybox.json

rootfs/docker_registry/docker_registry.tar: rootfs/docker_registry/docker_registry.json 
	packer build rootfs/docker_registry/docker_registry.json

rootfs/docker_registry_v2/docker_registry_v2.tar: rootfs/docker_registry_v2/docker_registry_v2.json rootfs/docker_registry_v2/provision.sh
	packer build rootfs/docker_registry_v2/docker_registry_v2.json

rootfs/fuse/fuse.tar: rootfs/fuse/fuse.json rootfs/fuse/provision.sh
	packer build rootfs/fuse/fuse.json

rootfs/preexisting_users/preexisting_users.tar: rootfs/preexisting_users/preexisting_users.json 
	packer build rootfs/preexisting_users/preexisting_users.json

rootfs/ubuntu/ubuntu.tar: rootfs/ubuntu/ubuntu.json 
	packer build rootfs/ubuntu/ubuntu.json

#
# Images
#

ubuntu: ${ROOTFS_TARS} ${GARDEN_CI_DEPS} garden-ci/Vagrantfile
	./rootfs/empty/scripts/compile_binary
	packer build garden-ci/garden-ci.json

ubuntu-docker: ${ROOTFS_TARS} ${GARDEN_CI_DEPS}
	./rootfs/empty/scripts/compile_binary
	packer build -only=docker garden-ci/garden-ci.json

ubuntu-vagrant: ${ROOTFS_TARS} ${GARDEN_CI_DEPS} garden-ci/Vagrantfile
	./rootfs/empty/scripts/compile_binary
	packer build -only=garden-ci-virtualbox-iso garden-ci/garden-ci.json


#
# Release targets
#

release-docker: 
	echo "About to push to Dockerhub"
	packer build -var-file garden-ci/version.json \
		           garden-ci/release_docker.json

release-vagrant:
	echo "About to push to Atlas"
	packer build -var-file garden-ci/version.json \
							 garden-ci/release_vagrant.json
