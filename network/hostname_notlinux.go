// +build !linux

package network

type hostNameSetter struct{}

func newHostname() *hostNameSetter {
	return &hostNameSetter{}
}

func (*hostNameSetter) SetHostname(hostName string) error {
	panic("not supported on this OS")
}
