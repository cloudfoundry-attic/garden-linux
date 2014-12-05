package network

import "github.com/pivotal-golang/lager"

func NewConfigurer(log lager.Logger) *Configurer {
	return &Configurer{
		Link:   Link{},
		Bridge: Bridge{},
		Veth:   VethCreator{},
		Logger: log,
	}
}
