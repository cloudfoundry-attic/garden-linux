package bridgemgr

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

type Destroyer interface {
	Destroy(name string) error
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
}

type mgr struct {
	names     BridgeNameGenerator
	destroyer Destroyer

	mu           sync.Mutex
	owners       map[string][]string // bridgeName -> []containerId
	bridgeSubnet map[string]string   // bridgeName -> subnet
	subnetBridge map[string]string   // subnet -> bridgeName
}

func New(prefix string, destroyer Destroyer) BridgeManager {
	return &mgr{
		names:     NewBridgeNameGenerator(prefix),
		destroyer: destroyer,

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
		return m.destroyer.Destroy(bridgeName)
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
		return fmt.Errorf("bridgepool: reacquired bridge name '%s' has already been acquired for subnet %s", bridgeName, sn)
	}

	m.subnetBridge[subnet.String()] = bridgeName
	m.owners[bridgeName] = append(m.owners[bridgeName], containerId)
	m.bridgeSubnet[bridgeName] = subnet.String()

	return nil
}

func remove(a []string, b string) []string {
	for i, j := range a {
		if j == b {
			return append(a[:i], a[i+1:]...)
		}
	}

	return a
}
