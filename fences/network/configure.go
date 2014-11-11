package network

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/fences/network/subnets"
	"github.com/docker/libcontainer/netlink"
)

// Pre-condition: the gateway IP is a valid IP in the subnet.
func ConfigureHost(hostInterface string, containerInterface string, netNsPid int, gatewayIP net.IP, subnet *net.IPNet, mtu int) error {
	return nil
}

var (
	ErrContainerInterfaceMissing = errors.New("container interface name must not be empty")
	ErrInvalidContainerIP        = errors.New("the container IP is not a valid address in the subnet")
	ErrInvalidGatewayIP          = errors.New("the gateway IP is not a valid address in the subnet")
	ErrConflictingIPs            = errors.New("the container IP must not be the same as the gateway IP")
	ErrSubnetNil                 = errors.New("subnet must be specified")
	ErrInvalidMtu                = errors.New("invalid MTU size")
	ErrBadContainerInterface     = errors.New("container interface not found")
	ErrFailedToAddIp             = errors.New("failed to add IP to interface")
	ErrFailedToAddGateway        = errors.New("failed to add gateway to interface")
	ErrFailedToLinkUp            = errors.New("failed to bring up the link")
	ErrFailedToSetMtu            = errors.New("failed to set MTU size on interface")
)

var InterfaceByName func(name string) (*net.Interface, error) = net.InterfaceByName
var NetworkLinkAddIp func(iface *net.Interface, ip net.IP, ipNet *net.IPNet) error = netlink.NetworkLinkAddIp
var AddDefaultGw func(ip, device string) error = netlink.AddDefaultGw
var NetworkLinkUp func(iface *net.Interface) error = netlink.NetworkLinkUp
var NetworkSetMTU func(iface *net.Interface, mtu int) error = netlink.NetworkSetMTU

// ConfigureContainer is called inside a network namespace to set the IP configuration for a container in a subnet.
// This function is non-atomic: if an error is returned the container configuration may be partially set.
func ConfigureContainer(containerInterface string, containerIP net.IP, gatewayIP net.IP, subnet *net.IPNet, mtu int) error {
	var err error
	if err = validateContainerConfiguration(containerInterface, containerIP, gatewayIP, subnet, mtu); err != nil {
		return err
	}

	var ifc *net.Interface

	if ifc, err = InterfaceByName(containerInterface); err != nil {
		return ErrBadContainerInterface // FIXME: need rich error type
	}

	if err = NetworkSetMTU(ifc, mtu); err != nil {
		return ErrFailedToSetMtu
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
