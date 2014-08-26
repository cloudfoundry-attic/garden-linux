package rootfs_provider

import (
	"net/url"

	"github.com/pivotal-golang/lager"
)

type RootFSProvider interface {
	ProvideRootFS(logger lager.Logger, id string, rootfs *url.URL) (mountpoint string, envvar []string, err error)
	CleanupRootFS(logger lager.Logger, id string) error
}
