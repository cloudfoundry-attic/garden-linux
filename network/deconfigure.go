package network

import (
	"errors"
	"fmt"

	"github.com/docker/libcontainer/netlink"
)

var (
	ErrFailedToDeleteBridgeInterface = errors.New("failed to delete bridge interface")
	ErrFailedToDeleteHostInterface   = errors.New("failed to delete host interface")
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
func DeconfigureHost(hostInterface Destroyer, bridgeInterface Destroyer) error {
	// FIXME: log this fmt.Printf("deconfigureHost(%q, %q)\n", hostInterface, bridgeInterface)
	if err := hostInterface.Destroy(); err != nil {
		if err.Error() != "no such network interface" {
			return ErrFailedToDeleteHostInterface // FIXME: rich error
		}
	}

	if bridgeInterface != nil {
		if err := bridgeInterface.Destroy(); err != nil {
			if err.Error() != "no such device" {
				return ErrFailedToDeleteBridgeInterface // FIXME: rich error
			}
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
