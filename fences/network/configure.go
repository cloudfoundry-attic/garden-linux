package network

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/fences/network/subnets"
	"github.com/docker/libcontainer/netlink"
	"github.com/milosgajdos83/tenus"
)

// Pre-condition: the gateway IP is a valid IP in the subnet.
func ConfigureHost(hostInterface string, containerInterface string, gatewayIP net.IP, subnetShareable bool, bridgeInterface string, subnet *net.IPNet, containerPid int, mtu int) error {
	err := netlink.NetworkCreateVethPair(hostInterface, containerInterface, 1)
	if err != nil {
		return ErrFailedToCreateVethPair // FIXME: need rich error type
	}

	var hostIfc *net.Interface
	if hostIfc, err = InterfaceByName(hostInterface); err != nil {
		return ErrBadHostInterface // FIXME: need rich error type
	}

	if err = NetworkSetMTU(hostIfc, mtu); err != nil {
		return ErrFailedToSetMtu // FIXME: need rich error type
	}

	if err = NetworkSetNsPid(hostIfc, 1); err != nil {
		return ErrFailedToSetHostNs // FIXME: need rich error type
	}

	var containerIfc *net.Interface
	if containerIfc, err = InterfaceByName(containerInterface); err != nil {
		return ErrBadContainerInterface // FIXME: need rich error type
	}

	if err = NetworkSetNsPid(containerIfc, containerPid); err != nil {
		return ErrFailedToSetContainerNs // FIXME: need rich error type
	}

	// FIXME: log this fmt.Println("---------------ConfigureHost: ", subnetShareable)

	if bridger, err := tenus.NewBridgeWithName(bridgeInterface); err == nil {
		if err = bridger.AddSlaveIfc(hostIfc); err != nil {
			return ErrFailedToAddSlave // FIXME: need rich error type
		}
		bridgeIfc := bridger.NetInterface()

		if err = NetworkLinkAddIp(bridgeIfc, gatewayIP, subnet); err != nil {
			return ErrFailedToAddIp // FIXME: need rich error type
		}

		if err = NetworkLinkUp(bridgeIfc); err != nil {
			return ErrFailedToLinkUp // FIXME: need rich error type
		}
	} else if !subnetShareable {
		// FIXME: log this fmt.Println("Failed to add bridge:", err)
		return ErrFailedToCreateBridge // FIXME: need rich error type

	} else {
		bridgeIfc, err := InterfaceByName(bridgeInterface)
		if err != nil {
			return ErrFailedToFindBridge // FIXME: rich error
		}
		err = netlink.AddToBridge(hostIfc, bridgeIfc)
		if err != nil {
			return ErrFailedToAddSlave // FIXME: need rich error type
		}
	}

	if err = NetworkLinkUp(hostIfc); err != nil {
		return ErrFailedToLinkUp // FIXME: need rich error type
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

//var NewBridgeWithName func(ifcName string) (tenus.Bridger, error) = tenus.NewBridgeWithName

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
