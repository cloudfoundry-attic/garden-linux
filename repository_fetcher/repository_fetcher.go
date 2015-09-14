package repository_fetcher

import (
	"errors"
	"io"
	"net/url"

	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/docker/distribution/digest"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_registry_provider/fake_registry_provider.go . RegistryProvider
type RegistryProvider interface {
	ProvideRegistry(hostname string) (*registry.Session, *registry.Endpoint, error)
}

//go:generate counterfeiter -o fake_lock/FakeLock.go . Lock
type Lock interface {
	Acquire(key string)
	Release(key string) error
}

// apes docker's *registry.Registry
type Registry interface {
	// v1 methods
	GetRepositoryData(repoName string) (*registry.RepositoryData, error)
	GetRemoteTags(registries []string, repository string) (map[string]string, error)
	GetRemoteHistory(imageID string, registry string) ([]string, error)
	GetRemoteImageJSON(imageID string, registry string) ([]byte, int, error)
	GetRemoteImageLayer(imageID string, registry string, size int64) (io.ReadCloser, error)

	// v2 methods
	GetV2ImageManifest(ep *registry.Endpoint, imageName, tagName string, auth *registry.RequestAuthorization) (digest.Digest, []byte, error)
	GetV2ImageBlobReader(ep *registry.Endpoint, imageName string, dgst digest.Digest, auth *registry.RequestAuthorization) (io.ReadCloser, int64, error)
}

type RemoteFetcher interface {
	Fetch(request *FetchRequest) (*FetchResponse, error)
}

type RepositoryFetcher interface {
	Fetch(logger lager.Logger, url *url.URL, diskQuota int64) (string, process.Env, []string, error)
}

type FetchRequest struct {
	Session    *registry.Session
	Endpoint   *registry.Endpoint
	Path       string
	RemotePath string
	Tag        string
	Logger     lager.Logger
	MaxSize    int64
}

type FetchResponse struct {
	ImageID string
	Env     process.Env
	Volumes []string
}

var ErrInvalidDockerURL = errors.New("invalid docker url")

// apes dockers registry.NewEndpoint
var RegistryNewEndpoint = registry.NewEndpoint

// apes dockers registry.NewSession
var RegistryNewSession = registry.NewSession
