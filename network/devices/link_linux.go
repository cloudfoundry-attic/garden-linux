package devices

import (
	"fmt"
	"net"

	"github.com/docker/libcontainer/netlink"
)

type Link struct{}

func (Link) AddIP(intf *net.Interface, ip net.IP, subnet *net.IPNet) error {
	return errF(netlink.NetworkLinkAddIp(intf, ip, subnet))
}

func (Link) AddDefaultGW(intf *net.Interface, ip net.IP) error {
	return errF(netlink.AddDefaultGw(ip.String(), intf.Name))
}

func (Link) SetUp(intf *net.Interface) error {
	return errF(netlink.NetworkLinkUp(intf))
}

func (Link) SetMTU(intf *net.Interface, mtu int) error {
	return errF(netlink.NetworkSetMTU(intf, mtu))
}

func (Link) SetNs(intf *net.Interface, ns int) error {
	return errF(netlink.NetworkSetNsPid(intf, ns))
}

func (Link) InterfaceByName(name string) (*net.Interface, bool, error) {
	intfs, err := net.Interfaces()
	if err != nil {
		return nil, false, errF(err)
	}

	for _, intf := range intfs {
		if intf.Name == name {
			return &intf, true, nil
		}
	}

	return nil, false, nil
}

func errF(err error) error {
	if err == nil {
		return err
	}

	return fmt.Errorf("devices: %v", err)
}
