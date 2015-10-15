default: all

all:
	go build -o linux_backend/bin/wshd github.com/cloudfoundry-incubator/garden-linux/containerizer/wshd
	go build -o linux_backend/skeleton/lib/pivotter github.com/cloudfoundry-incubator/garden-linux/containerizer/system/pivotter
	go build -o linux_backend/bin/iodaemon github.com/cloudfoundry-incubator/garden-linux/iodaemon/cmd/iodaemon
	go build -o linux_backend/bin/wsh github.com/cloudfoundry-incubator/garden-linux/container_daemon/wsh
	CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/bin/initc github.com/cloudfoundry-incubator/garden-linux/containerizer/initc
	CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/lib/hook github.com/cloudfoundry-incubator/garden-linux/hook/hook
	go build -o ${PWD}/out/garden-linux -tags daemon github.com/cloudfoundry-incubator/garden-linux
	cd linux_backend/src && make clean all
	cp linux_backend/src/oom/oom linux_backend/bin
	cp linux_backend/src/nstar/nstar linux_backend/bin
	cd linux_backend/src && make clean
	
.PHONY: default
