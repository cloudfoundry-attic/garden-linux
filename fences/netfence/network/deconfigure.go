package network

import (
	"net"

	"github.com/pivotal-golang/lager"
)

type Deconfigurer struct {
	Finder interface {
		InterfaceByName(name string) (*net.Interface, bool, error)
	}

	HostDeleter interface {
		Delete(bridge *net.Interface) error
	}

	BridgeDeleter interface {
		Delete(bridge *net.Interface) error
	}
}

// deconfigureHost undoes the effects of ConfigureHost.
// An empty bridge interface name should be specified if no bridge is to be deleted.
func (d *Deconfigurer) DeconfigureHost(log lager.Logger, hostIfc string, bridgeIfc string) error {
	log.Debug("destroy-host-ifc", lager.Data{"name": hostIfc})
	if err := d.deleteHost(hostIfc); err != nil {
		log.Error("destroy-host-ifc", err)
		return &DeleteLinkError{
			Cause: err,
			Role:  "host",
			Name:  hostIfc,
		}
	}

	if bridgeIfc == "" {
		return nil
	}

	log.Debug("destroy-bridge-ifc", lager.Data{"name": bridgeIfc})
	if err := d.deleteBridge(bridgeIfc); err != nil {
		log.Error("destroy-bridge-ifc", err)
		return &DeleteLinkError{
			Cause: err,
			Role:  "bridge",
			Name:  bridgeIfc,
		}
	}

	return nil
}

func (d *Deconfigurer) deleteHost(name string) error {
	if intf, found, err := d.Finder.InterfaceByName(name); err != nil || !found {
		return err
	} else {
		return d.HostDeleter.Delete(intf)
	}
}

func (d *Deconfigurer) deleteBridge(name string) error {
	if intf, found, err := d.Finder.InterfaceByName(name); err != nil || !found {
		return err
	} else {
		return d.BridgeDeleter.Delete(intf)
	}
}
