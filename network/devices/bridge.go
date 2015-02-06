package devices

import (
	"fmt"
	"net"

	"github.com/docker/libcontainer/netlink"
)

type Bridge struct{}

// Create creates a bridge device and returns the interface.
// If the device already exists, returns the existing interface.
func (Bridge) Create(name string, ip net.IP, subnet *net.IPNet) (intf *net.Interface, err error) {
	if err := netlink.NetworkLinkAdd(name, "bridge"); err != nil && err.Error() != "file exists" {
		return nil, fmt.Errorf("devices: create bridge: %v", err)
	}

	if intf, err = net.InterfaceByName(name); err != nil {
		return nil, fmt.Errorf("devices: look up created bridge interface: %v", err)
	}

	if err = netlink.NetworkLinkAddIp(intf, ip, subnet); err != nil && err.Error() != "file exists" {
		return nil, fmt.Errorf("devices: add IP to bridge: %v", err)
	}
	return intf, nil
}

func (Bridge) Add(bridge, slave *net.Interface) error {
	return netlink.AddToBridge(slave, bridge)
}

func (Bridge) Delete(bridge *net.Interface) error {
	return netlink.DeleteBridge(bridge.Name)
}
