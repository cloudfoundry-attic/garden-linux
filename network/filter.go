package network

import (
	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/gerr"
	"github.com/cloudfoundry-incubator/garden-linux/network/iptables"
)

//go:generate counterfeiter . Filter

type Filter interface {
	Setup() error
	TearDown()
	NetOut(garden.NetOutRule) error
}

type filter struct {
	chain iptables.Chain
}

func NewFilter(instanceChain iptables.Chain) Filter {
	return &filter{instanceChain}
}

func (fltr *filter) Setup() error {
	if err := fltr.chain.Setup(); err != nil {
		return gerr.NewFromError("log-chain-setup", err)
	}
	return nil
}

func (fltr *filter) TearDown() {
	fltr.chain.TearDown()
}

func (fltr *filter) NetOut(r garden.NetOutRule) error {
	return fltr.chain.PrependFilterRule(r)
}
