all: skeleton

skeleton:
	cd linux_backend/src && make clean all
	cp linux_backend/src/wsh/wshd linux_backend/skeleton/bin
	cp linux_backend/src/wsh/wsh linux_backend/skeleton/bin
	cp linux_backend/src/oom/oom linux_backend/skeleton/bin
	cp linux_backend/src/iomux/iomux-spawn linux_backend/skeleton/bin
	cp linux_backend/src/iomux/iomux-link linux_backend/skeleton/bin
	cp linux_backend/src/repquota/repquota linux_backend/bin

warden-test-rootfs.cid: integration/rootfs/Dockerfile
	docker build -t cloudfoundry/warden-test-rootfs --rm integration/rootfs
	docker run -cidfile=warden-test-rootfs.cid cloudfoundry/warden-test-rootfs echo

warden-test-rootfs.tar: warden-test-rootfs.cid
	docker export `cat warden-test-rootfs.cid` > warden-test-rootfs.tar
	docker rm `cat warden-test-rootfs.cid`
	rm warden-test-rootfs.cid

ci-image: warden-test-rootfs.tar
	docker build -t cloudfoundry/warden-ci --rm .
	rm warden-test-rootfs.tar
