// The subnets package provides a subnet pool from which networks may be dynamically acquired or
// statically reserved.
package subnets

import (
	"errors"
	"net"
	"sync"
)

// A Manager provides network allocations.
type Subnets interface {
	// Dynamically allocates a /30 subnet, or returns an error if no more subnets can be allocated by this manager
	AllocateDynamically() (*net.IPNet, error)

	// Statically allocates a /30 subnet, and returns an error if the address has already been allocated
	AllocateStatically(*net.IPNet) error

	// Releases a previously-allocated network back to the pool
	Release(*net.IPNet) error

	// Recover a previously allocated network so it appears to be allocated again
	Recover(*net.IPNet) error

	// Capacity -- number of /30 subnets in the dynamic allocation net
	Capacity() int
}

type subnetpool struct {
	dynamicAllocationNet *net.IPNet
	pool                 []*net.IPNet
	static               []*net.IPNet
	capacity             int

	mutex sync.Mutex
}

var (
	ErrInsufficientSubnets        = errors.New("Insufficient subnets remaining in the pool")
	ErrInvalidRange               = errors.New("Invalid IP Range")
	ErrReleasedUnallocatedNetwork = errors.New("cannot release an unallocated network")
	ErrAlreadyAllocated           = errors.New("Subnet has already been allocated")
)

var slash30mask net.IPMask

func init() {
	_, maskedNetwork, err := net.ParseCIDR("1.1.1.1/30")
	if err != nil {
		panic("Does not compute")
	}

	slash30mask = maskedNetwork.Mask
}

func New(ipNet *net.IPNet) (Subnets, error) {
	pool := poolOfSubnets(ipNet)
	return &subnetpool{dynamicAllocationNet: ipNet, pool: pool, capacity: len(pool)}, nil
}

func poolOfSubnets(ipNet *net.IPNet) []*net.IPNet {
	min := ipNet.IP
	pool := make([]*net.IPNet, 0)
	for ip := min; ipNet.Contains(ip); ip = next(ip) {
		subnet := &net.IPNet{ip, slash30mask}
		ip = next(next(next(ip)))
		if ipNet.Contains(ip) {
			pool = append(pool, subnet)
		}
	}

	return pool
}

func (m *subnetpool) Capacity() int {
	return m.capacity
}

func (m *subnetpool) AllocateStatically(ipNet *net.IPNet) error {
	if m.dynamicAllocationNet.Contains(ipNet.IP) {
		return ErrAlreadyAllocated
	}

	for _, s := range m.static {
		if s.Contains(ipNet.IP) {
			return ErrAlreadyAllocated
		}
	}

	m.static = append(m.static, ipNet)

	return nil
}

func (m *subnetpool) Recover(ipNet *net.IPNet) error {
	if !m.dynamicAllocationNet.Contains(ipNet.IP) {
		return m.AllocateStatically(ipNet)
	}

	found := -1
	for i, s := range m.pool {
		if s.IP.Equal(ipNet.IP) {
			found = i
		}
	}

	if found > -1 {
		m.pool = append(m.pool[:found], m.pool[found+1:]...)
		return nil
	}

	return ErrAlreadyAllocated
}

func (m *subnetpool) AllocateDynamically() (*net.IPNet, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(m.pool) == 0 {
		return nil, ErrInsufficientSubnets
	}

	acquired := m.pool[0]
	m.pool = m.pool[1:]

	return acquired, nil
}

func (m *subnetpool) Release(ipNet *net.IPNet) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, n := range m.pool {
		if n.IP.Equal(ipNet.IP) {
			return ErrReleasedUnallocatedNetwork
		}
	}

	found := -1
	for i, s := range m.static {
		if s.IP.Equal(ipNet.IP) {
			found = i
		}
	}

	if found > -1 {
		m.static = append(m.static[:found], m.static[found+1:]...)
	}

	m.pool = append(m.pool, ipNet)
	return nil
}

func next(ip net.IP) net.IP {
	next := clone(ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			return next
		}
	}

	panic("overflowed maximum IP")
}

func clone(ip net.IP) net.IP {
	clone := make([]byte, len(ip))
	copy(clone, ip)
	return clone
}
