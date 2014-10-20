package network

import (
	"encoding/json"
	"net"
)

type Network struct {
	ipNet *net.IPNet

	hostIP      net.IP
	containerIP net.IP
}

func New(ipNet *net.IPNet) *Network {
	return &Network{
		ipNet:       ipNet,
		hostIP:      maxValidIP(ipNet),
		containerIP: nextIP(ipNet.IP),
	}
}

func (n Network) IPNet() *net.IPNet {
	return n.ipNet
}

func (n Network) String() string {
	return n.ipNet.String()
}

func (n Network) IP() net.IP {
	return n.ipNet.IP
}

func (n Network) HostIP() net.IP {
	return n.hostIP
}

func (n Network) ContainerIP() net.IP {
	return n.containerIP
}

func (n Network) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"IPNet": n.String(),

		"HostIP":      n.HostIP(),
		"ContainerIP": n.ContainerIP(),
	})
}

func (n *Network) CIDRSuffix() int {
	suff, _ := n.ipNet.Mask.Size()
	return suff
}

func (n *Network) UnmarshalJSON(data []byte) error {
	var tmp struct {
		IPNet string

		HostIP      net.IP
		ContainerIP net.IP
	}

	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}

	_, ipNet, err := net.ParseCIDR(tmp.IPNet)
	if err != nil {
		return err
	}

	n.ipNet = ipNet
	n.hostIP = tmp.HostIP
	n.containerIP = tmp.ContainerIP

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
