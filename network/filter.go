package network

import (
	"fmt"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden-linux/network/iptables"
)

//go:generate counterfeiter . Filter

type Filter interface {
	Setup(logPrefix string) error
	TearDown()
	NetOut(garden.NetOutRule) error
}

type filter struct {
	chain iptables.Chain
}

func NewFilter(instanceChain iptables.Chain) Filter {
	return &filter{instanceChain}
}

func (fltr *filter) Setup(logPrefix string) error {
	if err := fltr.chain.Setup(logPrefix); err != nil {
		return fmt.Errorf("network: log chain setup: %v", err)
	}
	return nil
}

func (fltr *filter) TearDown() {
	fltr.chain.TearDown()
}

func (fltr *filter) NetOut(r garden.NetOutRule) error {
	return fltr.chain.PrependFilterRule(r)
}
