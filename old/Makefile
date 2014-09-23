all: skeleton

skeleton:
	GOPATH=${PWD}/../Godeps/_workspace:${GOPATH} go build -o linux_backend/skeleton/bin/iodaemon github.com/cloudfoundry-incubator/garden-linux/old/iodaemon
	cd linux_backend/src && make clean all
	cp linux_backend/src/wsh/wshd linux_backend/skeleton/bin
	cp linux_backend/src/wsh/wsh linux_backend/skeleton/bin
	cp linux_backend/src/oom/oom linux_backend/skeleton/bin
	cp linux_backend/src/nstar/nstar linux_backend/skeleton/bin
	cp linux_backend/src/repquota/repquota linux_backend/bin
	cd linux_backend/src && make clean
