package linux_backend

import (
	"net"
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/network/cnet"
)

type Resources struct {
	UserUID    uint32
	RootUID    uint32
	Network    cnet.ContainerNetwork
	Ports      []uint32
	ExternalIP net.IP

	portsLock *sync.Mutex
}

func NewResources(
	useruid uint32,
	rootuid uint32,
	network cnet.ContainerNetwork,
	ports []uint32,
	externalIP net.IP,
) *Resources {
	return &Resources{
		UserUID:    useruid,
		RootUID:    rootuid,
		Network:    network,
		Ports:      ports,
		ExternalIP: externalIP,

		portsLock: new(sync.Mutex),
	}
}

func (r *Resources) AddPort(port uint32) {
	r.portsLock.Lock()
	defer r.portsLock.Unlock()

	r.Ports = append(r.Ports, port)
}
