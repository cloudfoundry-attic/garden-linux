package network

import (
	"fmt"
	"net"
)

// VethPairCreationError is returned if creating a virtual ethernet pair fails
type VethPairCreationError struct {
	Cause                         error
	HostIfcName, ContainerIfcName string
}

func (err VethPairCreationError) Error() string {
	return fmtErr("failed to create veth pair with host interface name '%s', container interface name '%s': %v", err.HostIfcName, err.ContainerIfcName, err.Cause)
}

// MTUError is returned if setting the Mtu on an interface fails
type MTUError struct {
	Cause error
	Intf  *net.Interface
	MTU   int
}

func (err MTUError) Error() string {
	return fmtErr("failed to set interface '%v' mtu to %d", err.Intf, err.MTU, err.Cause)
}

type SetNsFailedError struct {
	Cause error
	Intf  *net.Interface
	Pid   int
}

func (err SetNsFailedError) Error() string {
	return ""
}

// BridgeCreationError is returned if an error occurs while creating a bridge
type BridgeCreationError struct {
	Cause  error
	Name   string
	IP     net.IP
	Subnet *net.IPNet
}

func (err BridgeCreationError) Error() string {
	return fmtErr("failed to create bridge with name '%s', IP '%s', subnet '%s': %v", err.Name, err.IP, err.Subnet, err.Cause)
}

// AddToBridgeError is returned if an error occurs while adding an interface to a bridge
type AddToBridgeError struct {
	Cause         error
	Bridge, Slave *net.Interface
}

func (err AddToBridgeError) Error() string {
	return fmtErr("failed to add slave %s to bridge %s: %v", err.Slave.Name, err.Bridge.Name, err.Cause)
}

// LinkUpError is returned if brinding an interface up fails
type LinkUpError struct {
	Cause error
	Link  *net.Interface
	Role  string
}

func (err LinkUpError) Error() string {
	return fmtErr("failed to bring %s link %s up: %v", err.Role, err.Link.Name, err.Cause)
}

func fmtErr(msg string, args ...interface{}) string {
	return fmt.Sprintf("network: "+msg, args...)
}
