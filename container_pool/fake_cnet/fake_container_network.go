package fake_cnet

import (
	"encoding/json"
	"net"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/network/cnet"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

type FakeBuilder struct {
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
	*FakeBuilder
}

func New(ipNet *net.IPNet) *FakeBuilder {
	return &FakeBuilder{
		ipNet: ipNet,

		nextNetwork: ipNet.IP,
	}
}

func (p *FakeBuilder) Build(spec string, sysconfig *sysconfig.Config, containerID string) (cnet.ContainerNetwork, error) {
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

func (p *FakeBuilder) Rebuild(rm *json.RawMessage) (cnet.ContainerNetwork, error) {
	if p.RebuildError != nil {
		return nil, p.RebuildError
	}

	p.Recovered = append(p.Recovered, string(*rm))
	return &FakeAllocation{p.ipNet.String(), p}, nil
}

func (p *FakeBuilder) Capacity() int {
	return p.InitialPoolSize
}

func (f *FakeAllocation) Deconfigure() error {
	return nil
}

func (b *FakeBuilder) Dismantle(ctrNetwork cnet.ContainerNetwork) error {
	f, ok := ctrNetwork.(*FakeAllocation)
	if !ok {
		panic("Unexpected concrete type for ContainerNetwork")
	}
	b.Released = append(b.Released, f.Subnet)
	return nil
}

func (b *FakeBuilder) ConfigureEnvironment(env process.Env) error {
	env["fake_global_env"] = "global_value"
	return nil
}

func (b *FakeBuilder) ExternalIP() net.IP {
	return nil
}

func (f *FakeAllocation) Info(i *garden.ContainerInfo) {
}

func (f *FakeAllocation) ConfigureEnvironment(env process.Env) error {
	env["fake_env"] = f.Subnet
	return nil
}

type FakeFlatCN struct {
	Subnet string
}

func (f *FakeAllocation) MarshalJSON() ([]byte, error) {
	if f.MarshalError != nil {
		return nil, f.MarshalError
	}
	if f.MarshalReturns != nil {
		return f.MarshalReturns, nil
	}
	ffcn := FakeFlatCN{f.FakeBuilder.Allocated[0]}
	return json.Marshal(ffcn)
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
