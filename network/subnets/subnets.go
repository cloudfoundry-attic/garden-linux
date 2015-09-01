// The subnets package provides a subnet pool from which networks may be dynamically acquired or
// statically reserved.
package subnets

import (
	"fmt"
	"math"
	"net"
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
)

// Subnets provides a means of allocating subnets and associated IP addresses.
type Subnets interface {
	// Allocates an IP address and associates it with a subnet. The subnet is selected by the given SubnetSelector.
	// The IP address is selected by the given IPSelector.
	// Returns a subnet, an IP address, and a boolean which is true if and only if this is the
	// first IP address to be associated with this subnet.
	// If either selector fails, an error is returned.
	Acquire(SubnetSelector, IPSelector) (*linux_backend.Network, error)

	// Releases an IP address associated with an allocated subnet. If the subnet has no other IP
	// addresses associated with it, it is deallocated.
	// Returns a boolean which is true if and only if the subnet was deallocated.
	// Returns an error if the given combination is not already in the pool.
	Release(*linux_backend.Network) error

	// Remove an IP address so it appears to be associated with the given subnet.
	Remove(*linux_backend.Network) error

	// Returns the number of /30 subnets which can be Acquired by a DynamicSubnetSelector.
	Capacity() int
}

type pool struct {
	allocated    map[string][]net.IP // net.IPNet.String +> seq net.IP
	dynamicRange *net.IPNet
	mu           sync.Mutex
}

//go:generate counterfeiter . SubnetSelector

// SubnetSelector is a strategy for selecting a subnet.
type SubnetSelector interface {
	// Returns a subnet based on a dynamic range and some existing statically-allocated
	// subnets. If no suitable subnet can be found, returns an error.
	SelectSubnet(dynamic *net.IPNet, existing []*net.IPNet) (*net.IPNet, error)
}

//go:generate counterfeiter . IPSelector

// IPSelector is a strategy for selecting an IP address in a subnet.
type IPSelector interface {
	// Returns an IP address in the given subnet which is not one of the given existing
	// IP addresses. If no such IP address can be found, returns an error.
	SelectIP(subnet *net.IPNet, existing []net.IP) (net.IP, error)
}

// New creates a Subnets implementation from a dynamic allocation range.
// All dynamic allocations come from the range, static allocations are prohibited
// from the dynamic range.
func NewSubnets(ipNet *net.IPNet) (Subnets, error) {
	return &pool{dynamicRange: ipNet, allocated: make(map[string][]net.IP)}, nil
}

// Acquire uses the given subnet and IP selectors to request a subnet, container IP address combination
// from the pool.
func (p *pool) Acquire(sn SubnetSelector, i IPSelector) (network *linux_backend.Network, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	network = &linux_backend.Network{}
	if network.Subnet, err = sn.SelectSubnet(p.dynamicRange, existingSubnets(p.allocated)); err != nil {
		return nil, err
	}

	ips := p.allocated[network.Subnet.String()]
	existingIPs := append(ips, NetworkIP(network.Subnet), GatewayIP(network.Subnet), BroadcastIP(network.Subnet))
	if network.IP, err = i.SelectIP(network.Subnet, existingIPs); err != nil {
		return nil, err
	}

	p.allocated[network.Subnet.String()] = append(ips, network.IP)
	return network, nil
}

// Recover re-allocates a given subnet and ip address combination in the pool. It returns
// an error if the combination is already allocated.
func (p *pool) Remove(network *linux_backend.Network) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if network.IP == nil {
		return ErrIpCannotBeNil
	}

	for _, existing := range p.allocated[network.Subnet.String()] {
		if existing.Equal(network.IP) {
			return ErrOverlapsExistingSubnet
		}
	}

	p.allocated[network.Subnet.String()] = append(p.allocated[network.Subnet.String()], network.IP)
	return nil
}

func (p *pool) Release(network *linux_backend.Network) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	subnetString := network.Subnet.String()
	ips := p.allocated[subnetString]

	if i, found := indexOf(ips, network.IP); found {
		if reducedIps, empty := removeIPAtIndex(ips, i); empty {
			delete(p.allocated, subnetString)
		} else {
			p.allocated[subnetString] = reducedIps
		}

		return nil
	}

	return ErrReleasedUnallocatedSubnet
}

// Capacity returns the number of /30 subnets that can be allocated
// from the pool's dynamic allocation range.
func (m *pool) Capacity() int {
	masked, total := m.dynamicRange.Mask.Size()
	return int(math.Pow(2, float64(total-masked)) / 4)
}

// Returns the gateway IP of a given subnet, which is always the maximum valid IP
func GatewayIP(subnet *net.IPNet) net.IP {
	return next(subnet.IP)
}

// Returns the network IP of a subnet.
func NetworkIP(subnet *net.IPNet) net.IP {
	return subnet.IP
}

// Returns the broadcast IP of a subnet.
func BroadcastIP(subnet *net.IPNet) net.IP {
	return max(subnet)
}

// returns the keys in the given map whose values are non-empty slices
func existingSubnets(m map[string][]net.IP) (result []*net.IPNet) {
	for k, v := range m {
		if len(v) > 0 {
			_, ipn, err := net.ParseCIDR(k)
			if err != nil {
				panic(fmt.Sprintf("failed to parse a CIDR in the subnet pool: %s", err))
			}

			result = append(result, ipn)
		}
	}

	return result
}

func indexOf(a []net.IP, w net.IP) (int, bool) {
	for i, v := range a {
		if v.Equal(w) {
			return i, true
		}
	}

	return -1, false
}

// removeAtIndex removes from a slice at the given index,
// and returns the new slice and boolean, true iff the new slice is empty.
func removeIPAtIndex(ips []net.IP, i int) ([]net.IP, bool) {
	l := len(ips)
	ips[i] = ips[l-1]
	ips = ips[:l-1]
	return ips, l == 1
}
