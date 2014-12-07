package network

import (
	"fmt"

	"github.com/docker/libcontainer/netlink"
	"github.com/pivotal-golang/lager"
)

type Destroyer interface {
	Destroy() error
}

type StringerDestroyer interface {
	fmt.Stringer
	Destroyer
}

// deconfigureHost undoes the effects of ConfigureHost.
// An empty bridge interface name should be specified if no bridge is to be deleted.
func DeconfigureHost(log lager.Logger, hostIfc StringerDestroyer, bridgeIfc StringerDestroyer) error {
	log.Debug("destroy-host-ifc", lager.Data{"name": hostIfc.String()})
	if err := hostIfc.Destroy(); err != nil && err.Error() != "no such network interface" {
		log.Error("destroy-host-ifc", err)
		return &DeleteLinkError{
			Cause: err,
			Role:  "host",
			Name:  hostIfc.String(),
		}
	}

	if bridgeIfc == nil {
		return nil
	}

	log.Debug("destroy-bridge-ifc", lager.Data{"name": bridgeIfc.String()})
	if err := bridgeIfc.Destroy(); err != nil && err.Error() != "no such network interface" {
		log.Error("destroy-bridge-ifc", err)
		return &DeleteLinkError{
			Cause: err,
			Role:  "bridge",
			Name:  bridgeIfc.String(),
		}
	}

	return nil
}

type DestroyableInterface string

func (d DestroyableInterface) Destroy() error {
	return netlink.NetworkLinkDel(string(d))
}

func (d DestroyableInterface) String() string {
	return string(d)
}

type DestroyableBridge string

func (d DestroyableBridge) Destroy() error {
	return netlink.DeleteBridge(string(d))
}

func (d DestroyableBridge) String() string {
	return string(d)
}
