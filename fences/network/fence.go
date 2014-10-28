package network

import (
	"encoding/json"
	"errors"
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

type FlatFence struct {
	Ipn         string
	ContainerIP string
}

var (
	ErrIPEqualsGateway   = errors.New("a container IP must not equal the gateway IP")
	ErrIPEqualsBroadcast = errors.New("a container IP must not equal the broadcast IP")
)

func (f *f) Build(spec string) (fences.Fence, error) {
	if spec == "" {
		ipn, err := f.Subnets.AllocateDynamically()
		if err != nil {
			return nil, err
		}

		containerIP := nextIP(ipn.IP)
		return &Allocation{ipn, containerIP, f}, nil
	} else {
		if !strings.Contains(spec, "/") {
			spec = spec + "/30"
		}

		specifiedIP, ipn, err := net.ParseCIDR(spec)
		if err != nil {
			return nil, err
		}

		if err := f.Subnets.AllocateStatically(ipn); err != nil {
			return nil, err
		}

		gatewayIP := maxValidIP(ipn)
		broadcastIP := nextIP(gatewayIP)

		var containerIP net.IP
		if specifiedIP.Equal(ipn.IP) {
			containerIP = nextIP(ipn.IP)
		} else if specifiedIP.Equal(gatewayIP) {
			return nil, ErrIPEqualsGateway
		} else if specifiedIP.Equal(broadcastIP) {
			return nil, ErrIPEqualsBroadcast
		} else {
			containerIP = specifiedIP
		}
		return &Allocation{ipn, containerIP, f}, nil
	}
}

func (f *f) Rebuild(rm *json.RawMessage) (fences.Fence, error) {
	ff := FlatFence{}
	if err := json.Unmarshal(*rm, &ff); err != nil {
		return nil, err
	}

	_, ipn, err := net.ParseCIDR(ff.Ipn)
	if err != nil {
		return nil, err
	}

	if err := f.Subnets.Recover(ipn); err != nil {
		return nil, err
	}

	return &Allocation{ipn, net.ParseIP(ff.ContainerIP), f}, nil
}

type Allocation struct {
	*net.IPNet
	containerIP net.IP
	parent      *f
}

func (a *Allocation) Dismantle() error {
	return a.parent.Release(a.IPNet)
}

func (a *Allocation) Info(i *api.ContainerInfo) {
	i.HostIP = maxValidIP(a.IPNet).String()
	i.ContainerIP = a.containerIP.String()
}

func (a *Allocation) MarshalJSON() ([]byte, error) {
	ff := FlatFence{a.IPNet.String(), a.containerIP.String()}
	return json.Marshal(ff)
}

func (a *Allocation) ConfigureProcess(env *[]string) error {
	suff, _ := a.IPNet.Mask.Size()

	*env = append(*env, fmt.Sprintf("network_host_ip=%s", maxValidIP(a.IPNet)),
		fmt.Sprintf("network_container_ip=%s", a.containerIP),
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
