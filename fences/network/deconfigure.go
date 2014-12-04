package network

import (
	"errors"

	"github.com/docker/libcontainer/netlink"
)

var (
	ErrFailedToDeleteBridgeInterface = errors.New("failed to delete bridge interface")
	ErrFailedToDeleteHostInterface   = errors.New("failed to delete host interface")
)

// deconfigureHost undoes the effects of ConfigureHost.
// An empty bridge interface name should be specified if no bridge is to be deleted.
func deconfigureHost(hostInterface Destroyer, bridgeInterface Destroyer) error {
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

type destroyableInterface string

func (d destroyableInterface) Destroy() error {
	return netlink.NetworkLinkDel(string(d))
}

func (d destroyableInterface) String() string {
	return string(d)
}

type destroyableBridge string

func (d destroyableBridge) Destroy() error {
	return netlink.DeleteBridge(string(d))
}

func (d destroyableBridge) String() string {
	return string(d)
}
