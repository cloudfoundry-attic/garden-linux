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
	// Returns the name of the bridge interface of the subnet.
	// Returns an error if the given combination is not already in the pool.
	Release(*net.IPNet, net.IP) (bool, string, error)

	// Recovers an IP address so it appears to be associated with the given subnet.
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
	return &bridgedSubnets{
		sn:   sn,
		bing: NewBridgeNameGenerator(prefix),
	}, nil
}

func (bs *bridgedSubnets) Allocate(ss SubnetSelector, is IPSelector) (*net.IPNet, net.IP, string, error) {
	panic("not implemented")
	bs.mu.Lock()
	defer bs.mu.Unlock()

	ipn, ip, first, err := bs.sn.Allocate(ss, is)
	if err != nil {
		return nil, nil, "", err
	}

	var bridgeIfcName string
	if first {
		bridgeIfcName = bs.bing.Generate()
		bs.bridgeNames[ipn.String()] = bridgeIfcName
	} else {
		var found bool
		bridgeIfcName, found = bs.bridgeNames[ipn.String()]
		if !found {
			panic("existing subnet must have a bridge interface name")
		}
	}

	return ipn, ip, bridgeIfcName, nil
}

func (bs *bridgedSubnets) Release(ipn *net.IPNet, ip net.IP) (bool, string, error) {
	panic("not implemented")
}

func (bs *bridgedSubnets) Recover(ipn *net.IPNet, ip net.IP, bridgeName string) error {
	panic("not implemented")
}

func (bs *bridgedSubnets) Capacity() int {
	panic("not implemented")
	return bs.sn.Capacity()
}
