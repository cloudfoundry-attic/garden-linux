// Package net_fence provides Garden's networking function.
package net_fence

import (
	"flag"
	"fmt"
	"github.com/cloudfoundry-incubator/garden-linux/net_fence/subnets"
	"net"
)

var config = struct {
	network string
}{}

const (
	DefaultNetworkPool = "10.254.0.0/22"
)

func InitializeFlags(flagset *flag.FlagSet) {
	flagset.StringVar(&config.network, "networkPool",
		DefaultNetworkPool,
		"Pool of subnets for dynamically allocated containers")
}

func Initialize() (subnets.Subnets, error) {
	_, network, err := net.ParseCIDR(config.network)
	if err != nil {
		return nil, fmt.Errorf("Invalid networkPool flag: %s", err)
	}

	return NewSubnets(network)
}

var NewSubnets = subnets.New
