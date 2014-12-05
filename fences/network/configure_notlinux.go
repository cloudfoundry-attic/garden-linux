// +build !linux

package network

func NewConfigurer() *Configurer {
	panic("not supported on this OS")
}
