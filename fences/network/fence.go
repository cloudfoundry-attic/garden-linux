package network

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/cloudfoundry-incubator/garden-linux/fences/network/subnets"
	"github.com/cloudfoundry-incubator/garden/api"
)

type f struct {
	subnets.Subnets
	mtu uint32
}

func (f *f) Build(spec string) (fences.Fence, error) {
	if spec == "" {
		ipn, err := f.Subnets.AllocateDynamically()
		if err != nil {
			return nil, err
		}

		return &Allocation{ipn, f}, nil
	} else {
		if !strings.Contains(spec, "/") {
			spec = spec + "/30"
		}

		_, ipn, err := net.ParseCIDR(spec)
		if err != nil {
			return nil, err
		}

		if err := f.Subnets.AllocateStatically(ipn); err != nil {
			return nil, err
		}

		return &Allocation{ipn, f}, nil
	}
}

func (f *f) Rebuild(rm *json.RawMessage) (fences.Fence, error) {
	var subnet string
	if err := json.Unmarshal(*rm, &subnet); err != nil {
		return nil, err
	}

	_, ipn, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, err
	}

	if err := f.Subnets.Recover(ipn); err != nil {
		return nil, err
	}

	return &Allocation{ipn, f}, nil
}

type Allocation struct {
	*net.IPNet
	parent *f
}

func (a *Allocation) Dismantle() error {
	return a.parent.Release(a.IPNet)
}

func (a *Allocation) Info(i *api.ContainerInfo) {
	i.HostIP = maxValidIP(a.IPNet).String()
	i.ContainerIP = nextIP(a.IPNet.IP).String()
}

func (a *Allocation) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.IPNet.String())
}

func (a *Allocation) ConfigureProcess(env *[]string) error {
	min := a.IPNet.IP
	suff, _ := a.IPNet.Mask.Size()

	*env = append(*env, fmt.Sprintf("network_host_ip=%s", maxValidIP(a.IPNet)),
		fmt.Sprintf("network_container_ip=%s", nextIP(min)),
		fmt.Sprintf("network_cidr_suffix=%d", suff),
		fmt.Sprintf("container_iface_mtu=%d", a.parent.mtu),
		fmt.Sprintf("network_cidr=%s", a.IPNet.String()))

	return nil
}

func maxValidIP(ipn *net.IPNet) net.IP {
	mask := ipn.Mask
	min := ipn.IP

	if len(mask) != len(min) {
		panic("length of mask is not compatible with length of network IP")
	}

	max := make([]byte, len(min))
	for i, b := range mask {
		max[i] = min[i] | ^b
	}

	// Do not include the network and broadcast addresses.
	max[len(max)-1]--

	return net.IP(max).To16()
}

func nextIP(ip net.IP) net.IP {
	next := net.ParseIP(ip.String())
	inc(next)
	return next
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
