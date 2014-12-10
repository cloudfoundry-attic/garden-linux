package network

import (
	"net"

	"github.com/pivotal-golang/lager"
)

type Deconfigurer struct {
	Finder interface {
		InterfaceByName(name string) (*net.Interface, bool, error)
	}

	BridgeDeleter interface {
		Delete(bridge *net.Interface) error
	}
}

// deconfigureHost undoes the effects of ConfigureHost.
// An empty bridge interface name should be specified if no bridge is to be deleted.
func (d *Deconfigurer) DeconfigureBridge(log lager.Logger, bridgeIfc string) error {
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

func (d *Deconfigurer) deleteBridge(name string) error {
	if intf, found, err := d.Finder.InterfaceByName(name); err != nil || !found {
		return err
	} else {
		return d.BridgeDeleter.Delete(intf)
	}
}
