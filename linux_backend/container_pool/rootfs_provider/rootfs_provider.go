package rootfs_provider

import "net/url"

type RootFSProvider interface {
	ProvideRootFS(id string, rootfs *url.URL) (mountpoint string, err error)
	CleanupRootFS(id string) error
}
