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

// deconfigureHost undoes the effects of ConfigureHost.
// An empty bridge interface name should be specified if no bridge is to be deleted.
func deconfigureHost(hostInterface string, bridgeInterface string) error {
	fmt.Printf("deconfigureHost(%q, %q)\n", hostInterface, bridgeInterface)
	if err := NetworkLinkDel(hostInterface); err != nil {
		if err.Error() != "no such network interface" {
			return ErrFailedToDeleteHostInterface // FIXME: rich error
		}
	}

	if bridgeInterface != "" {
		if err := DeleteBridge(bridgeInterface); err != nil {
			if err.Error() != "no such device" {
				return ErrFailedToDeleteBridgeInterface // FIXME: rich error
			}
		}
	}

	return nil
}
