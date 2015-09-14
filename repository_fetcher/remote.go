package repository_fetcher

import (
	"errors"
	"fmt"
	"net/url"
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

//go:generate counterfeiter -o fake_fetch_request_creator/fake_fetch_request_creator.go . FetchRequestCreator
type FetchRequestCreator interface {
	CreateFetchRequest(logger lager.Logger, repoURL *url.URL, diskQuota int64) (*FetchRequest, error)
}

type DockerRepositoryFetcher struct {
	requestCreator FetchRequestCreator
	fetchers       map[registry.APIVersion]VersionedFetcher
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
	size int64
}

func NewRemote(requestCreator FetchRequestCreator, fetchers map[registry.APIVersion]VersionedFetcher) RepositoryFetcher {
	return &DockerRepositoryFetcher{
		fetchers:       fetchers,
		requestCreator: requestCreator,
		fetchingLayers: map[string]chan struct{}{},
		fetchingMutex:  new(sync.Mutex),
	}
}

func FetchError(context, registry, reponame string, err error) error {
	return garden.NewServiceUnavailableError(fmt.Sprintf("repository_fetcher: %s: could not fetch image %s from registry %s: %s", context, reponame, registry, err))
}

func (fetcher *DockerRepositoryFetcher) Fetch(logger lager.Logger, repoURL *url.URL, diskQuota int64) (string, process.Env, []string, error) {
	errs := func(err error) (string, process.Env, []string, error) {
		return "", nil, nil, err
	}

	fetchRequest, err := fetcher.requestCreator.CreateFetchRequest(logger, repoURL, diskQuota)
	if err != nil {
		return errs(err)
	}

	if realFetcher, ok := fetcher.fetchers[fetchRequest.Endpoint.Version]; ok {
		response, err := realFetcher.Fetch(fetchRequest)
		if err != nil {
			return errs(err)
		}

		return response.ImageID, response.Env, response.Volumes, nil
	}

	return errs(errors.New("unknown docker registry API version"))
}
