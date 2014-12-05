package network

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/fences/network/subnets"
	"github.com/docker/libcontainer/netlink"
)

type Configurer struct {
	Veth interface {
		Create(hostIfcName, containerIfcName string) (*net.Interface, *net.Interface, error)
	}

	Link interface {
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

var (
	ErrBadContainerInterface     = errors.New("container interface not found")
	ErrBadHostInterface          = errors.New("host interface not found")
	ErrBadLoopbackInterface      = errors.New("cannot find the loopback interface")
	ErrConflictingIPs            = errors.New("the container IP must not be the same as the gateway IP")
	ErrContainerInterfaceMissing = errors.New("container interface name must not be empty")
	ErrFailedToAddGateway        = errors.New("failed to add gateway to interface")
	ErrFailedToAddIp             = errors.New("failed to add IP to interface")
	ErrFailedToAddLoopbackIp     = errors.New("failed to add IP to loopback interface")
	ErrFailedToAddSlave          = errors.New("failed to slave interface to bridge")
	ErrFailedToCreateBridge      = errors.New("failed to create bridge")
	ErrFailedToCreateVethPair    = errors.New("failed to create virtual ethernet pair")
	ErrFailedToFindBridge        = errors.New("failed to find bridge")
	ErrFailedToLinkUp            = errors.New("failed to bring up the link")
	ErrFailedToLinkUpLoopback    = errors.New("failed to bring up the loopback link")
	ErrFailedToSetContainerNs    = errors.New("failed to set container interface namespace")
	ErrFailedToSetHostNs         = errors.New("failed to set host interface namespace")
	ErrFailedToSetMtu            = errors.New("failed to set MTU size on interface")
	ErrInvalidContainerIP        = errors.New("the container IP is not a valid address in the subnet")
	ErrInvalidGatewayIP          = errors.New("the gateway IP is not a valid address in the subnet")
	ErrInvalidMtu                = errors.New("invalid MTU size")
	ErrSubnetNil                 = errors.New("subnet must be specified")
)

var InterfaceByName func(name string) (*net.Interface, error) = net.InterfaceByName
var NetworkLinkAddIp func(iface *net.Interface, ip net.IP, ipNet *net.IPNet) error = netlink.NetworkLinkAddIp
var AddDefaultGw func(ip, device string) error = netlink.AddDefaultGw
var NetworkLinkUp func(iface *net.Interface) error = netlink.NetworkLinkUp
var NetworkSetMTU func(iface *net.Interface, mtu int) error = netlink.NetworkSetMTU
var NetworkSetNsPid func(iface *net.Interface, nspid int) error = netlink.NetworkSetNsPid

// ConfigureContainer is called inside a network namespace to set the IP configuration for a container in a subnet.
// This function is non-atomic: if an error is returned the container configuration may be partially set.
func ConfigureContainer(containerInterface string, containerIP net.IP, gatewayIP net.IP, subnet *net.IPNet, mtu int) error {
	var err error
	if err = validateContainerConfiguration(containerInterface, containerIP, gatewayIP, subnet, mtu); err != nil {
		return err
	}

	var ifc *net.Interface

	// ip address add 127.0.0.1/8 dev lo
	// ip link set lo up
	if loIfc, err := InterfaceByName("lo"); err != nil {
		return ErrBadLoopbackInterface // FIXME: need rich error type
	} else {
		_, liIpNet, _ := net.ParseCIDR("127.0.0.1/8")
		if err = NetworkLinkAddIp(loIfc, net.ParseIP("127.0.0.1"), liIpNet); err != nil {
			return ErrFailedToAddLoopbackIp // FIXME: need rich error type
		}
		if err := NetworkLinkUp(loIfc); err != nil {
			return ErrFailedToLinkUpLoopback // FIXME: need rich error type
		}
	}

	if ifc, err = InterfaceByName(containerInterface); err != nil {
		return ErrBadContainerInterface // FIXME: need rich error type
	}

	if err = NetworkSetMTU(ifc, mtu); err != nil {
		return ErrFailedToSetMtu // FIXME: need rich error type
	}

	if err = NetworkLinkAddIp(ifc, containerIP, subnet); err != nil {
		return ErrFailedToAddIp // FIXME: need rich error type
	}

	if err = AddDefaultGw(gatewayIP.String(), containerInterface); err != nil {
		return ErrFailedToAddGateway // FIXME: need rich error type
	}

	if err = NetworkLinkUp(ifc); err != nil {
		return ErrFailedToLinkUp // FIXME: need rich error type
	}

	return nil
}

func validateContainerConfiguration(containerInterface string, containerIP net.IP, gatewayIP net.IP, subnet *net.IPNet, mtu int) error {
	if containerInterface == "" {
		return ErrContainerInterfaceMissing
	}

	if subnet == nil {
		return ErrSubnetNil
	}

	if !validIP(containerIP, subnet) {
		return ErrInvalidContainerIP
	}

	if !validIP(gatewayIP, subnet) {
		return ErrInvalidGatewayIP
	}

	if containerIP.Equal(gatewayIP) {
		return ErrConflictingIPs
	}

	if mtu <= 0 {
		return ErrInvalidMtu
	}

	return nil
}

func validIP(ip net.IP, subnet *net.IPNet) bool {
	return subnet.Contains(ip) && !subnet.IP.Equal(ip) && !subnets.BroadcastIP(subnet).Equal(ip)
}
