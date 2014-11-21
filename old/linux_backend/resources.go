package linux_backend

import (
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/fences"
)

type Resources struct {
	UserUID uint32
	RootUID uint32
	Network fences.Fence
	Ports   []uint32

	portsLock *sync.Mutex
}

func NewResources(
	useruid uint32,
	rootuid uint32,
	network fences.Fence,
	ports []uint32,
) *Resources {
	return &Resources{
		UserUID: useruid,
		RootUID: rootuid,
		Network: network,
		Ports:   ports,

		portsLock: new(sync.Mutex),
	}
}

func (r *Resources) AddPort(port uint32) {
	r.portsLock.Lock()
	defer r.portsLock.Unlock()

	r.Ports = append(r.Ports, port)
}
