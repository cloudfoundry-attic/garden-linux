// +build !linux

package network

import "code.cloudfoundry.org/lager"

func NewConfigurer(log lager.Logger) Configurer {
	panic("not supported on this OS")
}
