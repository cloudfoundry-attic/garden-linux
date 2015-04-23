all: skeleton

# Build hook without dynamic library dependencies. CGO_ENABLED=0 and the -a option achieve this.

skeleton:
	GOPATH=${PWD}/../Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/bin/wshd github.com/cloudfoundry-incubator/garden-linux/containerizer/wshd
	GOPATH=${PWD}/../Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/bin/initd github.com/cloudfoundry-incubator/garden-linux/containerizer/initd
	GOPATH=${PWD}/../Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/bin/wsh github.com/cloudfoundry-incubator/garden-linux/container_daemon/wsh
	GOPATH=${PWD}/../Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/bin/iodaemon github.com/cloudfoundry-incubator/garden-linux/iodaemon
	GOPATH=${PWD}/../Godeps/_workspace:${GOPATH} CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/lib/hook github.com/cloudfoundry-incubator/garden-linux/hook/hook
	cd linux_backend/src && make clean all
	cp linux_backend/src/oom/oom linux_backend/skeleton/bin
	cp linux_backend/src/nstar/nstar linux_backend/skeleton/bin
	cp linux_backend/src/repquota/repquota linux_backend/bin
	cd linux_backend/src && make clean
