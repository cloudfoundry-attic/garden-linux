package network

import "github.com/pivotal-golang/lager"

import "github.com/cloudfoundry-incubator/garden-linux/network/devices"

func NewConfigurer(log lager.Logger) Configurer {
	return &NetworkConfigurer{
		Hostname: newHostname(),
		Link:     devices.Link{},
		Bridge:   devices.Bridge{},
		Veth:     devices.VethCreator{},
		Logger:   log,
	}
}
