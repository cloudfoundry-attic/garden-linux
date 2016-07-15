package bridgemgr

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"code.cloudfoundry.org/garden-linux/network/subnets"
)

type Builder interface {
	Create(name string, ip net.IP, subnet *net.IPNet) (intf *net.Interface, err error)
	Destroy(name string) error
}

type Lister interface {
	List() ([]string, error)
}

//go:generate counterfeiter -o fake_bridge_manager/FakeBridgeManager.go . BridgeManager
type BridgeManager interface {
	// Reserve reserves a bridge name for a subnet.
	// if this is the first call of 'reserve' for a subnet created a new, unique bridge name
	Reserve(subnet *net.IPNet, containerId string) (string, error)

	// Rereserves adds a container to the list of reservations for a particular bridge name.
	Rereserve(bridgeName string, subnet *net.IPNet, containerId string) error

	// Release releases a reservation made by a particular container.
	// If this is the last reservation, the passed destroyers Destroy method is called.
	Release(bridgeName string, containerId string) error

	// Prune deletes all bridges starting with prefix, that are unknown.
	Prune() error
}

type mgr struct {
	prefix  string
	names   BridgeNameGenerator
	builder Builder
	lister  Lister

	mu           sync.Mutex
	owners       map[string][]string // bridgeName -> []containerId
	bridgeSubnet map[string]string   // bridgeName -> subnet
	subnetBridge map[string]string   // subnet -> bridgeName
}

func New(prefix string, builder Builder, lister Lister) BridgeManager {
	return &mgr{
		prefix:  prefix,
		names:   NewBridgeNameGenerator(prefix),
		builder: builder,
		lister:  lister,

		owners:       make(map[string][]string),
		bridgeSubnet: make(map[string]string),
		subnetBridge: make(map[string]string),
	}
}

func (m *mgr) Reserve(subnet *net.IPNet, containerId string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	name, present := m.subnetBridge[subnet.String()]

	if !present {
		name = m.names.Generate()
		if _, err := m.builder.Create(name, subnets.GatewayIP(subnet), subnet); err != nil {
			return "", err
		}
		m.subnetBridge[subnet.String()] = name
		m.bridgeSubnet[name] = subnet.String()
	}

	m.owners[name] = append(m.owners[name], containerId)

	return name, nil
}

func (m *mgr) Release(bridgeName string, containerId string) error {
	m.mu.Lock()
	m.owners[bridgeName] = remove(m.owners[bridgeName], containerId)

	shouldDelete := false
	if len(m.owners[bridgeName]) == 0 {
		delete(m.owners, bridgeName)
		delete(m.subnetBridge, m.bridgeSubnet[bridgeName])
		delete(m.bridgeSubnet, bridgeName)
		shouldDelete = true
	}

	m.mu.Unlock()

	if shouldDelete {
		return m.builder.Destroy(bridgeName)
	}

	return nil
}

func (m *mgr) Rereserve(bridgeName string, subnet *net.IPNet, containerId string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if bridgeName == "" {
		return errors.New("bridgemgr: re-reserving bridge: bridge name must not be empty")
	}

	if sn, present := m.bridgeSubnet[bridgeName]; present && sn != subnet.String() {
		return fmt.Errorf("bridgemgr: reacquired bridge name '%s' has already been acquired for subnet %s", bridgeName, sn)
	}

	m.subnetBridge[subnet.String()] = bridgeName
	m.owners[bridgeName] = append(m.owners[bridgeName], containerId)
	m.bridgeSubnet[bridgeName] = subnet.String()

	return nil
}

func (m *mgr) Prune() error {
	list, err := m.lister.List()
	if err != nil {
		return fmt.Errorf("bridgemgr: pruning bridges: %v", err)
	}

	for _, b := range list {
		if !strings.HasPrefix(b, m.prefix) {
			continue
		}

		if !m.isReserved(b) {
			m.builder.Destroy(b)
		}
	}

	return nil
}

func (m *mgr) isReserved(r string) bool {
	_, ok := m.bridgeSubnet[r]
	return ok
}

func remove(a []string, b string) []string {
	for i, j := range a {
		if j == b {
			return append(a[:i], a[i+1:]...)
		}
	}

	return a
}
