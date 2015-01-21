package network

import (
	"errors"
	"fmt"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/iptables"
)

//go:generate counterfeiter . Filter

type Filter interface {
	Setup() error
	TearDown()
	NetOut(network string, port uint32, portRange string, protocol garden.Protocol, icmpType int32, icmpCode int32, log bool) error
}

type filter struct {
	chain iptables.Chain
}

func NewFilter(instanceChain iptables.Chain) Filter {
	return &filter{instanceChain}
}

func (fltr *filter) Setup() error {
	if err := fltr.chain.Setup(); err != nil {
		return fmt.Errorf("network: log chain setup: %v", err)
	}
	return nil
}

func (fltr *filter) TearDown() {
	fltr.chain.TearDown()
}

func (fltr *filter) NetOut(network string, port uint32, portRange string, protocol garden.Protocol, icmpType int32, icmpCode int32, log bool) error {
	if protocol != garden.ProtocolICMP && (icmpType != -1 || icmpCode != -1) {
		return errors.New("invalid rule: icmp code or icmp type can only be specified with protocol ICMP")
	}
	if network == "" && port == 0 && portRange == "" {
		return errors.New("invalid rule: either network or port (range) must be specified")
	}
	if (port != 0 || portRange != "") && !fltr.protocolAllowsPortFiltering(protocol) {
		return errors.New("invalid rule: a port (range) can only be specified with protocol TCP or UDP")
	}
	if port != 0 && portRange != "" {
		return errors.New("invalid rule: port and port range cannot both be specified")
	}
	return fltr.chain.PrependFilterRule(protocol, network, port, portRange, icmpType, icmpCode, log)
}

func (fltr *filter) protocolAllowsPortFiltering(protocol garden.Protocol) bool {
	return protocol == garden.ProtocolTCP || protocol == garden.ProtocolUDP
}
