package network

import (
	"errors"
	"net"
)

type Configurer struct {
	Veth interface {
		Create(hostIfcName, containerIfcName string) (*net.Interface, *net.Interface, error)
	}

	Link interface {
		AddIP(intf *net.Interface, ip net.IP, subnet *net.IPNet) error
		AddDefaultGW(intf *net.Interface, ip net.IP) error
		SetUp(intf *net.Interface) error
		SetMTU(intf *net.Interface, mtu int) error
		SetNs(intf *net.Interface, pid int) error
		InterfaceByName(name string) (*net.Interface, bool, error)
	}

	Bridge interface {
		Create(bridgeName string, ip net.IP, subnet *net.IPNet) (*net.Interface, error)
		Add(bridge, slave *net.Interface) error
	}
}

func (c *Configurer) ConfigureHost(hostIfcName, containerIfcName, bridgeName string, containerPid int, bridgeIP net.IP, subnet *net.IPNet, mtu int) error {
	var (
		err       error
		host      *net.Interface
		container *net.Interface
		bridge    *net.Interface
	)

	if bridge, err = c.configureBridgeIntf(bridgeName, bridgeIP, subnet); err != nil {
		return err
	}

	if host, container, err = c.configureVethPair(hostIfcName, containerIfcName); err != nil {
		return err
	}

	if err = c.configureHostIntf(host, bridge, mtu); err != nil {
		return err
	}

	// move container end in to container
	if err = c.Link.SetNs(container, containerPid); err != nil {
		return &SetNsFailedError{err, container, containerPid}
	}

	return nil
}

func (c *Configurer) configureBridgeIntf(name string, ip net.IP, subnet *net.IPNet) (*net.Interface, error) {
	bridge, bridgeExists, err := c.Link.InterfaceByName(name)
	if err != nil {
		return nil, &BridgeCreationError{errors.New("look up existing bridge"), name, ip, subnet}
	}

	if !bridgeExists {
		if bridge, err = c.Bridge.Create(name, ip, subnet); err != nil {
			return nil, &BridgeCreationError{err, name, ip, subnet}
		}
	}

	if err = c.Link.SetUp(bridge); err != nil {
		return nil, &LinkUpError{err, bridge, "bridge"}
	}

	return bridge, nil
}

func (c *Configurer) configureVethPair(hostName, containerName string) (*net.Interface, *net.Interface, error) {
	if host, container, err := c.Veth.Create(hostName, containerName); err != nil {
		return nil, nil, &VethPairCreationError{err, hostName, containerName}
	} else {
		return host, container, err
	}
}

func (c *Configurer) configureHostIntf(intf *net.Interface, bridge *net.Interface, mtu int) error {
	if err := c.Link.SetMTU(intf, mtu); err != nil {
		return &MTUError{err, intf, mtu}
	}

	if err := c.Bridge.Add(bridge, intf); err != nil {
		return &AddToBridgeError{err, bridge, intf}
	}

	if err := c.Link.SetUp(intf); err != nil {
		return &LinkUpError{err, intf, "host"}
	}

	return nil
}

func (c *Configurer) ConfigureContainer(containerIntf string, containerIP net.IP, gatewayIP net.IP, subnet *net.IPNet, mtu int) error {
	if err := c.configureLoopbackIntf(); err != nil {
		return err
	}

	if err := c.configureContainerIntf(containerIntf, containerIP, gatewayIP, subnet, mtu); err != nil {
		return err
	}

	return nil
}

func (c *Configurer) configureContainerIntf(name string, ip, gatewayIP net.IP, subnet *net.IPNet, mtu int) (err error) {
	var found bool
	var intf *net.Interface
	if intf, found, err = c.Link.InterfaceByName(name); !found || err != nil {
		return &FindLinkError{err, "container", name}
	}

	if err := c.Link.AddIP(intf, ip, subnet); err != nil {
		return &ConfigureLinkError{err, "container", intf, ip, subnet}
	}

	if err := c.Link.SetUp(intf); err != nil {
		return &LinkUpError{err, intf, "container"}
	}

	if err := c.Link.AddDefaultGW(intf, gatewayIP); err != nil {
		return &ConfigureDefaultGWError{err, intf, gatewayIP}
	}

	if err := c.Link.SetMTU(intf, mtu); err != nil {
		return &MTUError{err, intf, mtu}
	}

	return nil
}

func (c *Configurer) configureLoopbackIntf() (err error) {
	var found bool
	var lo *net.Interface
	if lo, found, err = c.Link.InterfaceByName("lo"); !found || err != nil {
		return &FindLinkError{err, "loopback", "lo"}
	}

	ip, subnet, err := net.ParseCIDR("127.0.0.1/8")
	if err != nil {
		panic("can't parse 127.0.0.1/8 as a CIDR") // cant happen
	}

	if err := c.Link.AddIP(lo, ip, subnet); err != nil {
		return &ConfigureLinkError{err, "loopback", lo, ip, subnet}
	}

	if err := c.Link.SetUp(lo); err != nil {
		return &LinkUpError{err, lo, "loopback"}
	}

	return nil
}
