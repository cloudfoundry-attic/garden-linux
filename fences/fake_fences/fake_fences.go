package fake_fences

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
)

type FakeFences struct {
	ipNet       *net.IPNet
	nextNetwork net.IP

	InitialPoolSize int

	RebuildError  error
	AllocateError error
	MarshalError  error

	MarshalReturns []byte

	Released  []string
	Recovered []string
	Allocated []string
}

type FakeAllocation struct {
	Subnet string
	*FakeFences
}

func New(ipNet *net.IPNet) *FakeFences {
	return &FakeFences{
		ipNet: ipNet,

		nextNetwork: ipNet.IP,
	}
}

func (p *FakeFences) Build(spec string, sysconfig *sysconfig.Config, containerID string) (fences.Fence, error) {
	if spec == "" {
		spec = "1.2.0.0/30"
	}

	_, ipn, err := net.ParseCIDR(spec)
	if err != nil {
		return nil, err
	}

	if p.AllocateError != nil {
		return nil, p.AllocateError
	}

	p.Allocated = append(p.Allocated, spec)
	return &FakeAllocation{ipn.String(), p}, nil
}

func (p *FakeFences) Rebuild(rm *json.RawMessage) (fences.Fence, error) {
	if p.RebuildError != nil {
		return nil, p.RebuildError
	}

	p.Recovered = append(p.Recovered, string(*rm))
	return &FakeAllocation{p.ipNet.String(), p}, nil
}

func (p *FakeFences) Capacity() int {
	return p.InitialPoolSize
}

func (f *FakeAllocation) Deconfigure() error {
	return nil
}

func (f *FakeAllocation) Dismantle() error {
	f.FakeFences.Released = append(f.FakeFences.Released, f.Subnet)
	return nil
}

func (f *FakeAllocation) Info(i *garden.ContainerInfo) {
}

func (f *FakeAllocation) ConfigureProcess(env *[]string) error {
	*env = append(*env, fmt.Sprintf("fake_fences_env=%s", f.Subnet))
	return nil
}

type FakeFlatFence struct {
	Subnet string
}

func (f *FakeAllocation) MarshalJSON() ([]byte, error) {
	if f.MarshalError != nil {
		return nil, f.MarshalError
	}
	if f.MarshalReturns != nil {
		return f.MarshalReturns, nil
	}
	ff := FakeFlatFence{f.FakeFences.Allocated[0]}
	return json.Marshal(ff)
}

func (f *FakeAllocation) String() string {
	return "fake allocation"
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
