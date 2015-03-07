all: skeleton

# Build container-net without dynamic library dependencies. CGO_ENABLED=0 and the -a option achieve this.

skeleton:
	GOPATH=${PWD}/../Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/bin/iodaemon github.com/cloudfoundry-incubator/garden-linux/iodaemon
	GOPATH=${PWD}/../Godeps/_workspace:${GOPATH} CGO_ENABLED=0 go build -a -o linux_backend/skeleton/bin/container-net github.com/cloudfoundry-incubator/garden-linux/network/container-net
	GOPATH=${PWD}/../Godeps/_workspace:${GOPATH} CGO_ENABLED=0 go build -a -installsuffix static -o linux_backend/skeleton/lib/hook github.com/cloudfoundry-incubator/garden-linux/hook/hook
	cd linux_backend/src && make clean all
	cp linux_backend/src/wsh/wshd linux_backend/skeleton/bin
	cp linux_backend/src/wsh/wsh linux_backend/skeleton/bin
	cp linux_backend/src/oom/oom linux_backend/skeleton/bin
	cp linux_backend/src/nstar/nstar linux_backend/skeleton/bin
	cp linux_backend/src/repquota/repquota linux_backend/bin
	cd linux_backend/src && make clean
