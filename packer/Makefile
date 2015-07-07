all: ubuntu

clean:
	rm -fr build
	find rootfs -name '*.tar' -exec rm {} \;
	find . -type d -name 'output' -exec rm -rf {} \;

ubuntu: rootfs-tars
	packer build garden-ci/garden-ci.json

ubuntu-docker: rootfs-tars
	packer build -only=docker garden-ci/garden-ci.json

ubuntu-vagrant:
	packer build -only=garden-ci-virtualbox-iso garden-ci/garden-ci.json

ubuntu-ami: rootfs-tars
	packer build -only=amazon-ebs garden-ci/garden-ci.json

rootfs-tars:
	./rootfs/empty/scripts/compile_binary
	find rootfs -name '*.json' -exec packer build {} \;

release-docker:
	echo "About to push to Dockerhub"
	packer build -var-file garden-ci/version.json \
		           garden-ci/release_docker.json

release-vagrant:
	echo "About to push to Atlas"
	packer build -var-file garden-ci/version.json \
							 garden-ci/release_vagrant.json
