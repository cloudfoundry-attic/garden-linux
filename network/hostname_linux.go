package network

import "syscall"

type hostNameSetter struct{}

func newHostname() *hostNameSetter {
	return &hostNameSetter{}
}

func (*hostNameSetter) SetHostname(hostname string) error {
	return syscall.Sethostname([]byte(hostname))
}
