package rootfs_provider

import (
	"net/url"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_rootfs_provider/fake_rootfs_provider.go . RootFSProvider
type RootFSProvider interface {
	Name() string
	ProvideRootFS(logger lager.Logger, id string, rootfs *url.URL, namespaced bool, quota int64) (mountpoint string, envvar process.Env, err error)
}

//go:generate counterfeiter -o fake_graph/fake_graph.go . Graph
type Graph interface {
	layercake.Cake
}
