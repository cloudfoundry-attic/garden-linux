package repository_fetcher

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_versioned_fetcher/fake_versioned_fetcher.go . VersionedFetcher
type VersionedFetcher interface {
	Fetch(*FetchRequest) (*FetchResponse, error)
}

//go:generate counterfeiter -o fake_pinger/fake_pinger.go . Pinger
type Pinger interface {
	Ping(*registry.Endpoint) (registry.RegistryInfo, error)
}

type EndpointPinger struct{}

func (EndpointPinger) Ping(e *registry.Endpoint) (registry.RegistryInfo, error) {
	return e.Ping()
}

type DockerRepositoryFetcher struct {
	fetchers map[registry.APIVersion]VersionedFetcher

	registryProvider RegistryProvider
	pinger           Pinger
	graph            Graph

	fetchingLayers map[string]chan struct{}
	fetchingMutex  *sync.Mutex
}

type dockerImage struct {
	layers []*dockerLayer
}

func (d dockerImage) Env() process.Env {
	envs := process.Env{}
	for _, l := range d.layers {
		envs = envs.Merge(l.env)
	}

	return envs
}

func (d dockerImage) Vols() []string {
	var vols []string
	for _, l := range d.layers {
		vols = append(vols, l.vols...)
	}

	return vols
}

type dockerLayer struct {
	env  process.Env
	vols []string
}

func NewRemote(provider RegistryProvider, graph Graph, fetchers map[registry.APIVersion]VersionedFetcher, pinger Pinger) RepositoryFetcher {
	return &DockerRepositoryFetcher{
		fetchers:         fetchers,
		registryProvider: provider,
		pinger:           pinger,
		graph:            graph,
		fetchingLayers:   map[string]chan struct{}{},
		fetchingMutex:    new(sync.Mutex),
	}
}

func FetchError(context, registry, reponame string, err error) error {
	return garden.NewServiceUnavailableError(fmt.Sprintf("repository_fetcher: %s: could not fetch image %s from registry %s: %s", context, reponame, registry, err))
}

func (fetcher *DockerRepositoryFetcher) Fetch(
	logger lager.Logger,
	repoURL *url.URL,
	tag string,
) (string, process.Env, []string, error) {
	errs := func(err error) (string, process.Env, []string, error) {
		return "", nil, nil, err
	}

	fLog := logger.Session("fetch", lager.Data{
		"repo": repoURL,
		"tag":  tag,
	})

	fLog.Debug("fetching")

	if len(repoURL.Path) == 0 {
		return errs(ErrInvalidDockerURL)
	}

	path := repoURL.Path[1:]

	r, endpoint, err := fetcher.registryProvider.ProvideRegistry(repoURL.Host)
	if err != nil {
		logger.Error("failed-to-construct-registry-endpoint", err)
		return errs(FetchError("ProvideRegistry", repoURL.Host, path, err))
	}

	var regInfo registry.RegistryInfo
	if regInfo, err = fetcher.pinger.Ping(endpoint); err == nil {
		logger.Debug("pinged-registry", lager.Data{
			"info":             regInfo,
			"endpoint-version": endpoint.Version,
		})
	} else {
		return errs(err)
	}

	remotePath := path
	if !regInfo.Standalone && strings.IndexRune(remotePath, '/') == -1 {
		remotePath = "library/" + remotePath
	}

	fetchRequest := &FetchRequest{
		Session:    r,
		Endpoint:   endpoint,
		Logger:     fLog,
		Path:       path,
		RemotePath: remotePath,
		Tag:        tag,
	}

	if realFetcher, ok := fetcher.fetchers[endpoint.Version]; ok {
		response, err := realFetcher.Fetch(fetchRequest)
		if err != nil {
			return errs(err)
		}

		return response.ImageID, response.Env, response.Volumes, nil
	}

	return errs(errors.New("unknown docker registry API version"))
}
