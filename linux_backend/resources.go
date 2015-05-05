package linux_backend

import (
	"encoding/json"
	"net"
	"sync"
)

type Network struct {
	Subnet *net.IPNet
	IP     net.IP
}

func (n *Network) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"IP":     n.IP.String(),
		"Subnet": n.Subnet.String(),
	})
}

func (n *Network) UnmarshalJSON(b []byte) error {
	var u = struct {
		IP     string
		Subnet string
	}{}

	if err := json.Unmarshal(b, &u); err != nil {
		return err
	}

	var err error
	n.IP = net.ParseIP(u.IP)
	_, n.Subnet, err = net.ParseCIDR(u.Subnet)
	return err
}

type Resources struct {
	UserUID    int
	RootUID    int
	Network    *Network
	Bridge     string
	Ports      []uint32
	ExternalIP net.IP

	portsLock *sync.Mutex
}

func NewResources(
	useruid int,
	rootuid int,
	network *Network,
	bridge string,
	ports []uint32,
	externalIP net.IP,
) *Resources {
	return &Resources{
		UserUID:    useruid,
		RootUID:    rootuid,
		Bridge:     bridge,
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
