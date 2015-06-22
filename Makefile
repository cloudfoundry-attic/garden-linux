default: all

all:
	GOPATH=${PWD}/Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/bin/wshd github.com/cloudfoundry-incubator/garden-linux/containerizer/wshd
	GOPATH=${PWD}/Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/lib/pivotter github.com/cloudfoundry-incubator/garden-linux/containerizer/system/pivotter
	GOPATH=${PWD}/Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/bin/iodaemon github.com/cloudfoundry-incubator/garden-linux/iodaemon
	GOPATH=${PWD}/Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/bin/wsh github.com/cloudfoundry-incubator/garden-linux/container_daemon/wsh
	GOPATH=${PWD}/Godeps/_workspace:${GOPATH} CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/bin/initc github.com/cloudfoundry-incubator/garden-linux/containerizer/initc
	GOPATH=${PWD}/Godeps/_workspace:${GOPATH} CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/bin/initd github.com/cloudfoundry-incubator/garden-linux/container_daemon/initd
	GOPATH=${PWD}/Godeps/_workspace:${GOPATH} CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/lib/proc_starter github.com/cloudfoundry-incubator/garden-linux/container_daemon/proc_starter
	GOPATH=${PWD}/Godeps/_workspace:${GOPATH} CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/lib/hook github.com/cloudfoundry-incubator/garden-linux/hook/hook
	GOPATH=${PWD}/Godeps/_workspace:${GOPATH} go build -o ${PWD}/out/garden-linux -tags daemon github.com/cloudfoundry-incubator/garden-linux
	cd linux_backend/src && make clean all
	cp linux_backend/src/oom/oom linux_backend/skeleton/bin
	cp linux_backend/src/nstar/nstar linux_backend/skeleton/bin
	cd linux_backend/src && make clean
	
.PHONY: default
