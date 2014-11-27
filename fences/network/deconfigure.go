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

var (
	NetworkLinkDel func(name string) error = netlink.NetworkLinkDel
	DeleteBridge   func(name string) error = netlink.DeleteBridge
)

// DeconfigureHost undoes the effects of ConfigureHost.
func DeconfigureHost(hostInterface string, bridgeInterface string) error {
	if err := NetworkLinkDel(hostInterface); err != nil {
		if err.Error() != "no such network interface" {
			return ErrFailedToDeleteHostInterface // FIXME: rich error
		}
	}

	if err := DeleteBridge(bridgeInterface); err != nil {
		if err.Error() != "no such device" {
			return ErrFailedToDeleteBridgeInterface // FIXME: rich error
		}
	}

	return nil
}
