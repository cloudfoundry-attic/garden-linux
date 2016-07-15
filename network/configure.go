package network

import (
	"errors"
	"net"

	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter . Configurer
type Configurer interface {
	ConfigureContainer(*ContainerConfig) error
	ConfigureHost(*HostConfig) error
}

//go:generate counterfeiter . Hostname
type Hostname interface {
	SetHostname(hostName string) error
}

type NetworkConfigurer struct {
	Hostname Hostname

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

	Logger lager.Logger
}

type HostConfig struct {
	HostIntf      string
	BridgeName    string
	BridgeIP      net.IP
	ContainerIntf string
	ContainerPid  int
	Subnet        *net.IPNet
	Mtu           int
}

func (c *NetworkConfigurer) ConfigureHost(config *HostConfig) error {
	var (
		err       error
		host      *net.Interface
		container *net.Interface
		bridge    *net.Interface
	)

	cLog := c.Logger.Session("configure-host", lager.Data{
		"bridgeName":     config.BridgeName,
		"bridgeIP":       config.BridgeIP,
		"subnet":         config.Subnet,
		"containerIface": config.ContainerIntf,
		"hostIface":      config.HostIntf,
		"mtu":            config.Mtu,
		"pid":            config.ContainerPid,
	})

	cLog.Debug("configuring")

	if bridge, err = c.configureBridgeIntf(cLog, config.BridgeName, config.BridgeIP, config.Subnet); err != nil {
		return err
	}

	if host, container, err = c.configureVethPair(cLog, config.HostIntf, config.ContainerIntf); err != nil {
		return err
	}

	if err = c.configureHostIntf(cLog, host, bridge, config.Mtu); err != nil {
		return err
	}

	// move container end in to container
	if err = c.Link.SetNs(container, config.ContainerPid); err != nil {
		return &SetNsFailedError{err, container, config.ContainerPid}
	}

	return nil
}

func (c *NetworkConfigurer) configureBridgeIntf(log lager.Logger, name string, ip net.IP, subnet *net.IPNet) (*net.Interface, error) {
	log = log.Session("bridge-interface")

	log.Debug("find")
	bridge, bridgeExists, err := c.Link.InterfaceByName(name)
	if err != nil || !bridgeExists {
		log.Error("find", err)
		return nil, &BridgeDetectionError{errors.New("look up existing bridge"), name, ip, subnet}
	}

	log.Debug("bring-up")
	if err = c.Link.SetUp(bridge); err != nil {
		log.Error("bring-up", err)
		return nil, &LinkUpError{err, bridge, "bridge"}
	}

	return bridge, nil
}

func (c *NetworkConfigurer) configureVethPair(log lager.Logger, hostName, containerName string) (*net.Interface, *net.Interface, error) {
	log = log.Session("veth")

	log.Debug("create")
	if host, container, err := c.Veth.Create(hostName, containerName); err != nil {
		log.Error("create", err)
		return nil, nil, &VethPairCreationError{err, hostName, containerName}
	} else {
		return host, container, err
	}
}

func (c *NetworkConfigurer) configureHostIntf(log lager.Logger, intf *net.Interface, bridge *net.Interface, mtu int) error {
	log = log.Session("host-interface", lager.Data{
		"bridge-interface": bridge,
		"host-interface":   intf,
	})

	log.Debug("set-mtu")
	if err := c.Link.SetMTU(intf, mtu); err != nil {
		log.Error("set-mtu", err)
		return &MTUError{err, intf, mtu}
	}

	log.Debug("add-to-bridge")
	if err := c.Bridge.Add(bridge, intf); err != nil {
		log.Error("add-to-bridge", err)
		return &AddToBridgeError{err, bridge, intf}
	}

	log.Debug("bring-link-up")
	if err := c.Link.SetUp(intf); err != nil {
		log.Error("bring-link-up", err)
		return &LinkUpError{err, intf, "host"}
	}

	return nil
}

type ContainerConfig struct {
	Hostname      string
	ContainerIntf string
	ContainerIP   net.IP
	GatewayIP     net.IP
	Subnet        *net.IPNet
	Mtu           int
}

func (c *NetworkConfigurer) ConfigureContainer(config *ContainerConfig) error {
	if err := c.configureLoopbackIntf(); err != nil {
		return err
	}

	if err := c.configureContainerIntf(
		config.ContainerIntf,
		config.ContainerIP,
		config.GatewayIP,
		config.Subnet,
		config.Mtu,
	); err != nil {
		return err
	}

	return c.Hostname.SetHostname(config.Hostname)
}

func (c *NetworkConfigurer) configureContainerIntf(name string, ip, gatewayIP net.IP, subnet *net.IPNet, mtu int) (err error) {
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

func (c *NetworkConfigurer) configureLoopbackIntf() (err error) {
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
