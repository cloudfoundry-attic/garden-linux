package rootfs_provider

import (
	"net/url"

	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_rootfs_provider/fake_rootfs_provider.go . RootFSProvider
type RootFSProvider interface {
	ProvideRootFS(logger lager.Logger, id string, rootfs *url.URL) (mountpoint string, envvar process.Env, err error)
	CleanupRootFS(logger lager.Logger, id string) error
}
