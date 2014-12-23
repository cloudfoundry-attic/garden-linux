package network

import (
	"fmt"

	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/iptables"
	"github.com/cloudfoundry-incubator/garden/api"
)

type FilterFactory interface {
	Create(id string) Filter
}

type Filter interface {
	NetOut(network string, port uint32, protocol api.Protocol) error
}

type filterFactory struct {
	instancePrefix string
	chainFactory   iptables.ChainFactory
}

type filter struct {
	instanceChain iptables.Chain
}

func NewFilterFactory(tag string, chainFactory iptables.ChainFactory) FilterFactory {
	return &filterFactory{instancePrefix: fmt.Sprintf("w-%s-instance-", tag),
		chainFactory: chainFactory,
	}
}

func (ff *filterFactory) Create(id string) Filter {
	return &filter{instanceChain: ff.chainFactory.CreateChain(ff.instancePrefix + id)}
}

func (fltr *filter) NetOut(network string, port uint32, protocol api.Protocol) error {
	return fltr.instanceChain.PrependFilterRule(protocol, network, port)
}
