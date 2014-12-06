// The subnets package provides a subnet pool from which networks may be dynamically acquired or
// statically reserved.
package subnets

import (
	"fmt"
	"math"
	"net"
	"sync"
)

// Subnets provides a means of allocating subnets.
type Subnets interface {
	// Allocates a subnet and container IP address. The subnet is selected by the given SubnetSelector.
	// The IP address is selected by the given IPSelector. If either selector fails, an error is returned.
	Allocate(SubnetSelector, IPSelector) (*net.IPNet, net.IP, error)

	// Releases an allocated network and container IP.
	// Return a boolean which is true if and only if the network is no longer in use by other containers.
	// Returns an error if the given combination is not already in the pool.
	Release(*net.IPNet, net.IP) (bool, error)

	// Recovers an unallocated subnet and container IP so they appear to be allocated.
	Recover(*net.IPNet, net.IP) error

	// Returns the number of /30 subnets which can be Allocated by a DynamicSubnetSelector.
	Capacity() int
}

type pool struct {
	allocated    map[string][]net.IP // net.IPNet.String +> seq net.IP
	dynamicRange *net.IPNet
	mu           sync.Mutex
}

// SubnetSelector is a strategy for selecting a subnet.
type SubnetSelector interface {
	// Returns a subnet based on a dynamic range and some existing statically-allocated
	// subnets. If no suitable subnet can be found, returns an error.
	SelectSubnet(dynamic *net.IPNet, existing []*net.IPNet) (*net.IPNet, error)
}

// IPSelector is a strategy for selecting an IP address in a subnet.
type IPSelector interface {
	// Returns an IP address in the given subnet which is not one of the given existing
	// IP addresses. If no such IP address can be found, returns an error.
	SelectIP(subnet *net.IPNet, existing []net.IP) (net.IP, error)
}

// New creates a Subnets implementation from a dynamic allocation range.
// All dynamic allocations come from the range, static allocations are prohibited
// from the dynamic range.
func New(ipNet *net.IPNet) (Subnets, error) {
	return &pool{dynamicRange: ipNet, allocated: make(map[string][]net.IP)}, nil
}

// Allocate uses the given subnet and IP selectors to request a subnet, container IP address combination
// from the pool.
func (p *pool) Allocate(sn SubnetSelector, i IPSelector) (subnet *net.IPNet, ip net.IP, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if subnet, err = sn.SelectSubnet(p.dynamicRange, existingSubnets(p.allocated)); err != nil {
		return nil, nil, err
	}

	existingIPs := append(p.allocated[subnet.String()], NetworkIP(subnet), GatewayIP(subnet), BroadcastIP(subnet))
	if ip, err = i.SelectIP(subnet, existingIPs); err != nil {
		return nil, nil, err
	}

	p.allocated[subnet.String()] = append(p.allocated[subnet.String()], ip)
	return subnet, ip, nil
}

// Recover re-allocates a given subnet and ip address combination in the pool. It returns
// an error if the combination is already allocated.
func (p *pool) Recover(subnet *net.IPNet, ip net.IP) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ip == nil {
		return ErrIpCannotBeNil
	}

	for _, existing := range p.allocated[subnet.String()] {
		if existing.Equal(ip) {
			return ErrOverlapsExistingSubnet
		}
	}

	p.allocated[subnet.String()] = append(p.allocated[subnet.String()], ip)
	return nil
}

func (p *pool) Release(subnet *net.IPNet, containerIP net.IP) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if i, found := indexOf(p.allocated[subnet.String()], containerIP); found {
		return removeAtIndex(p.allocated, subnet.String(), i), nil
	}

	return false, ErrReleasedUnallocatedSubnet
}

// Capacity returns the number of /30 subnets that can be allocated
// from the pool's dynamic allocation range.
func (m *pool) Capacity() int {
	masked, total := m.dynamicRange.Mask.Size()
	return int(math.Pow(2, float64(total-masked)) / 4)
}

// Returns the gateway IP of a given subnet, which is always the maximum valid IP
func GatewayIP(subnet *net.IPNet) net.IP {
	m := max(subnet)
	m[len(m)-1]--

	return m
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

// removeAtIndex removes from the array at the given key and index, and returns true if the array is then empty
func removeAtIndex(m map[string][]net.IP, key string, i int) (removedAll bool) {
	m[key] = append(m[key][:i], m[key][i+1:]...)
	return len(m[key]) == 0
}
