package network

import (
	"errors"
	"fmt"

	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/iptables"
	"github.com/cloudfoundry-incubator/garden/api"
)

type FilterFactory interface {
	Create(id string) Filter
}

type Filter interface {
	NetOut(network string, port uint32, portRange string, protocol api.Protocol) error
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

func (fltr *filter) NetOut(network string, port uint32, portRange string, protocol api.Protocol) error {
	if network == "" && port == 0 && portRange == "" {
		return errors.New("invalid rule: either network or port must be specified")
	}
	if (port != 0 || portRange != "") && protocol != api.ProtocolTCP {
		return errors.New("invalid rule: a port (range) can only be specified with protocol TCP")
	}
	if port != 0 && portRange != "" {
		return errors.New("invalid rule: port and port range cannot both be specified")
	}
	return fltr.instanceChain.PrependFilterRule(protocol, network, port, portRange)
}
