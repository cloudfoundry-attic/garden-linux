// +build !linux

package network

import "github.com/pivotal-golang/lager"

func NewConfigurer(log lager.Logger) Configurer {
	panic("not supported on this OS")
}
