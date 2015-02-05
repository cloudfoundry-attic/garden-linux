package subnets

import (
	"net"
	"sync"
)

//go:generate counterfeiter . BridgedSubnets

// BridgedSubnets provides a means of allocating subnets, associated IP addresses, and bridge interface names.
type BridgedSubnets interface {
	// Allocates an IP address and associates it with a subnet. The subnet is selected by the given SubnetSelector.
	// The IP address is selected by the given IPSelector.
	// Returns a subnet, an IP address, and the name of the bridge interface of the subnet.
	// If either selector fails, an error is returned.
	Allocate(SubnetSelector, IPSelector) (*net.IPNet, net.IP, string, error)

	// Releases an IP address associated with an allocated subnet. If the subnet has no other IP
	// addresses associated with it, it is deallocated.
	// Returns a boolean which is true if and only if the subnet was deallocated.
	// Returns the name of the bridge interface name which was associated with the subnet.
	// Returns an error if the given combination is not already in the pool.
	Release(*net.IPNet, net.IP) (bool, string, error)

	// Recovers an IP address so it appears to be associated with the given subnet and bridge interface name.
	Recover(*net.IPNet, net.IP, string) error

	// Returns the number of /30 subnets which can be Allocated by a DynamicSubnetSelector.
	Capacity() int
}

type bridgedSubnets struct {
	sn          Subnets
	bing        BridgeNameGenerator
	mu          sync.Mutex
	bridgeNames map[string]string
}

// NewBridgedSubnets creates a BridgedSubnets implementation from a dynamic allocation range
// and a bridge prefix.
// All dynamic allocations come from the range, static allocations are prohibited
// from the dynamic range.
func NewBridgedSubnets(ipNet *net.IPNet, prefix string) (BridgedSubnets, error) {
	sn, err := NewSubnets(ipNet)
	if err != nil {
		return nil, err
	}
	return NewBridgedSubnetsWithDelegates(sn, NewBridgeNameGenerator(prefix)), nil
}

// NewBridgedSubnetsWithDelegates creates a BridgedSubnets implementation from a Subnets
// instance and a bridge name generator.
func NewBridgedSubnetsWithDelegates(sn Subnets, bing BridgeNameGenerator) BridgedSubnets {
	return &bridgedSubnets{
		sn:          sn,
		bing:        bing,
		bridgeNames: make(map[string]string),
	}
}

func (bs *bridgedSubnets) Allocate(ss SubnetSelector, is IPSelector) (*net.IPNet, net.IP, string, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	ipn, ip, first, err := bs.sn.Allocate(ss, is)
	if err != nil {
		return nil, nil, "", err
	}

	var bridgeIfcName string
	if first {
		if bname, present := bs.bridgeNames[ipn.String()]; present {
			panic("cannot add a bridge name when one already exists: " + bname)
		}
		bridgeIfcName = bs.bing.Generate()
		bs.bridgeNames[ipn.String()] = bridgeIfcName
	} else {
		bridgeIfcName = bs.subnetBridgeName(ipn)
	}

	return ipn, ip, bridgeIfcName, nil
}

func (bs *bridgedSubnets) Release(ipn *net.IPNet, ip net.IP) (bool, string, error) {
	validateIPNet(ipn)
	bs.mu.Lock()
	defer bs.mu.Unlock()

	last, err := bs.sn.Release(ipn, ip)
	if err != nil {
		return last, "", err
	}

	bridgeName := bs.subnetBridgeName(ipn)
	if last {
		delete(bs.bridgeNames, ipn.String())
	}
	return last, bridgeName, nil
}

func (bs *bridgedSubnets) Recover(ipn *net.IPNet, ip net.IP, bridgeName string) error {
	validateIPNet(ipn)
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if err := bs.sn.Recover(ipn, ip); err != nil {
		return err
	}

	bs.bridgeNames[ipn.String()] = bridgeName

	return nil
}

func (bs *bridgedSubnets) Capacity() int {
	return bs.sn.Capacity()
}

func (bs *bridgedSubnets) subnetBridgeName(ipn *net.IPNet) string {
	bridgeIfcName, found := bs.bridgeNames[ipn.String()]
	if !found {
		panic("existing subnet must have a bridge interface name")
	}
	return bridgeIfcName
}

func validateIPNet(ipn *net.IPNet) {
	if ipn == nil {
		panic("*net.IPNet parameter must not be nil")
	}
}
