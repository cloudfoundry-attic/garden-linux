package network

import "code.cloudfoundry.org/lager"

import "code.cloudfoundry.org/garden-linux/network/devices"

func NewConfigurer(log lager.Logger) Configurer {
	return &NetworkConfigurer{
		Hostname: newHostname(),
		Link:     devices.Link{},
		Bridge:   devices.Bridge{},
		Veth:     devices.VethCreator{},
		Logger:   log,
	}
}
