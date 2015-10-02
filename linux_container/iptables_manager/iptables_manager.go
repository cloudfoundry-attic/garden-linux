package iptables_manager

import "net"

//go:generate counterfeiter -o fake_chain/fake_chain.go . Chain
type Chain interface {
	Setup(containerID, bridgeName string, ip net.IP, network *net.IPNet) error
	Teardown(containerID string) error
}

type IPTablesManager struct {
	chains []Chain
}

func New() *IPTablesManager {
	return &IPTablesManager{}
}

func (mgr *IPTablesManager) AddChain(chain Chain) *IPTablesManager {
	mgr.chains = append(mgr.chains, chain)

	return mgr
}

func (mgr *IPTablesManager) ContainerSetup(containerID, bridgeName string, ip net.IP, network *net.IPNet) error {
	if err := mgr.ContainerTeardown(containerID); err != nil {
		return err
	}

	for index, chain := range mgr.chains {
		if err := chain.Setup(containerID, bridgeName, ip, network); err != nil {
			for i := 0; i < index; i++ {
				mgr.chains[i].Teardown(containerID)
			}
			return err
		}

	}

	return nil
}

func (mgr *IPTablesManager) ContainerTeardown(containerID string) error {
	var lastErr error
	for _, chain := range mgr.chains {
		if err := chain.Teardown(containerID); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
