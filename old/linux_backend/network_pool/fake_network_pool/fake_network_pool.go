package fake_network_pool

import (
	"net"
)

type FakeNetworkPool struct {
	ipNet       *net.IPNet
	nextNetwork net.IP

	InitialPoolSize int

	AcquireError        error
	RecoverError        error
	AllocateStaticError error

	Released            []string
	Recovered           []string
	StaticallyAllocated []string
}

func New(ipNet *net.IPNet) *FakeNetworkPool {
	return &FakeNetworkPool{
		ipNet: ipNet,

		nextNetwork: ipNet.IP,
	}
}

func (p *FakeNetworkPool) Capacity() int {
	return p.InitialPoolSize
}

func (p *FakeNetworkPool) AllocateDynamically() (*net.IPNet, error) {
	if p.AcquireError != nil {
		return nil, p.AcquireError
	}

	_, ipNet, err := net.ParseCIDR(p.nextNetwork.String() + "/30")
	if err != nil {
		return nil, err
	}

	inc(p.nextNetwork)
	inc(p.nextNetwork)
	inc(p.nextNetwork)
	inc(p.nextNetwork)

	return ipNet, nil
}

func (p *FakeNetworkPool) AllocateStatically(ipNet *net.IPNet) error {
	if p.AllocateStaticError != nil {
		return p.AllocateStaticError
	}

	p.StaticallyAllocated = append(p.StaticallyAllocated, ipNet.String())
	return nil
}

func (p *FakeNetworkPool) PoolFoxNetworkBad() string {
	return p.ipNet.String()
}

func (p *FakeNetworkPool) Release(ipNet *net.IPNet) error {
	p.Released = append(p.Released, ipNet.String())
	return nil
}

func (p *FakeNetworkPool) Recover(ipNet *net.IPNet) error {
	if p.RecoverError != nil {
		return p.RecoverError
	}

	p.Recovered = append(p.Recovered, ipNet.String())
	return nil
}

func (p *FakeNetworkPool) Network() *net.IPNet {
	return p.ipNet
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
