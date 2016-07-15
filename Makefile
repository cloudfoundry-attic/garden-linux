default: all

all:
	go build -o linux_backend/bin/wshd code.cloudfoundry.org/garden-linux/containerizer/wshd
	go build -o linux_backend/skeleton/lib/pivotter code.cloudfoundry.org/garden-linux/containerizer/system/pivotter
	go build -o linux_backend/bin/iodaemon code.cloudfoundry.org/garden-linux/iodaemon/cmd/iodaemon
	go build -o linux_backend/bin/wsh code.cloudfoundry.org/garden-linux/container_daemon/wsh
	CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/bin/initc code.cloudfoundry.org/garden-linux/containerizer/initc
	CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/lib/hook code.cloudfoundry.org/garden-linux/hook/hook
	go build -o ${PWD}/out/garden-linux -tags daemon code.cloudfoundry.org/garden-linux
	cd linux_backend/src && make clean all
	cp linux_backend/src/oom/oom linux_backend/skeleton/bin
	cp linux_backend/src/nstar/nstar linux_backend/bin
	cd linux_backend/src && make clean
	
.PHONY: default
