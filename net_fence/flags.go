// Package net_fence provides Garden's networking function.
package net_fence

import (
	"flag"
	"fmt"
	"math"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/net_fence/subnets"
)

var config = struct {
	network string
	mtu     uint64
}{}

const (
	DefaultNetworkPool        = "10.254.0.0/22"
	DefaultMTUSize     uint32 = 1500
)

func InitializeFlags(flagset *flag.FlagSet) {
	flagset.StringVar(&config.network, "networkPool", DefaultNetworkPool,
		"Pool of dynamically allocated container subnets")

	flagset.Uint64Var(&config.mtu, "mtu", uint64(DefaultMTUSize),
		"MTU size for container network interfaces")
}

func Initialize() (subnets.Subnets, uint32, error) {
	_, network, err := net.ParseCIDR(config.network)
	if err != nil {
		return nil, 0, fmt.Errorf("Invalid networkPool flag: %s", err)
	}
	subnets, err := NewSubnets(network)
	if err != nil {
		return nil, 0, err
	}

	if config.mtu > math.MaxUint32 {
		return nil, 0, fmt.Errorf("Invalid value %d for flag -mtu: value out of range (maximum value %d)", config.mtu, math.MaxUint32)
	}
	mtu := uint32(config.mtu)

	return subnets, mtu, nil
}

var NewSubnets = subnets.New
