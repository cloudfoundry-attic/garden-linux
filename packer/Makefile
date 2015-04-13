all: ubuntu

clean:
	rm -fr build
	find rootfs -name '*.tar' -exec rm {} \;
	find . -type d -name 'output' -exec rm -rf {} \;

ubuntu: rootfs-tars
	packer build garden-ci/garden-ci.json

ubuntu-docker: rootfs-tars
	packer build -only=docker garden-ci/garden-ci.json

ubuntu-vagrant: rootfs-tars
	packer build -except=docker garden-ci/garden-ci.json

rootfs-tars:
	./rootfs/empty/scripts/compile_binary
	find rootfs -name '*.json' -exec packer build {} \;

GARDEN_PACKER_DOCKER_REPO ?= cloudfoundry/garden-ci-ubuntu
docker_repo = $(GARDEN_PACKER_DOCKER_REPO)
docker_image_version = $$(cat garden-ci/DOCKER_IMAGE_VERSION)
release-docker:
	echo "About to push $(docker_repo):$(docker_image_version) to docker hub"
	packer build -var "docker_repository=$(docker_repo)" \
	             -var "image_version=$(docker_image_version)" \
		           garden-ci/release_docker.json

GARDEN_PACKER_VAGRANT_BOX_TAG ?= cloudfoundry/garden-ci-ubuntu
vagrant_box_tag = $(GARDEN_PACKER_VAGRANT_BOX_TAG)
vagrant_box_version = $$(cat garden-ci/VAGRANT_VIRTUALBOX_VERSION)
release-vagrant:
	echo "About to push $(vagrant_box_tag) v$(vagrant_box_version) to Atlas"
	packer build -var "box_tag=$(vagrant_box_tag)" \
	             -var "box_version=$(vagrant_box_version)" \
		           garden-ci/release_vagrant.json

