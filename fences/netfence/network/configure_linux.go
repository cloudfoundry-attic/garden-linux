package network

import "github.com/pivotal-golang/lager"
import "github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/devices"

func NewConfigurer(log lager.Logger) *Configurer {
	return &Configurer{
		Link:   devices.Link{},
		Bridge: devices.Bridge{},
		Veth:   devices.VethCreator{},
		Logger: log,
	}
}
