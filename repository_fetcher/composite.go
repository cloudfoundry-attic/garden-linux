package repository_fetcher

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/docker/docker/registry"
)

//go:generate counterfeiter -o fake_versioned_fetcher/fake_versioned_fetcher.go . VersionedFetcher
type VersionedFetcher interface {
	Fetch(*FetchRequest) (*Image, error)
	FetchID(*FetchRequest) (layercake.ID, error)
}

type CompositeFetcher struct {
	// fetcher used for requests without a scheme
	LocalFetcher RepositoryFetcher

	// fetchers used for docker:// urls, depending on the version
	RemoteFetchers map[registry.APIVersion]VersionedFetcher

	// creates remote fetch requests for a particular URL
	RequestCreator FetchRequestCreator
}

//go:generate counterfeiter -o fake_fetch_request_creator/fake_fetch_request_creator.go . FetchRequestCreator
type FetchRequestCreator interface {
	CreateFetchRequest(repoURL *url.URL, diskQuota int64) (*FetchRequest, error)
}

func (f *CompositeFetcher) Fetch(repoURL *url.URL, diskQuota int64) (*Image, error) {
	if repoURL.Scheme == "" {
		return f.LocalFetcher.Fetch(repoURL, diskQuota)
	}

	req, err := f.RequestCreator.CreateFetchRequest(repoURL, diskQuota)
	if err != nil {
		return nil, err
	}

	fetcher, ok := f.RemoteFetchers[req.Endpoint.Version]
	if !ok {
		return nil, errors.New("repository_fetcher: fetching an image: incompatible endpoint version")
	}

	return fetcher.Fetch(req)
}

func (f *CompositeFetcher) FetchID(repoURL *url.URL) (layercake.ID, error) {
	if repoURL.Scheme == "" {
		return f.LocalFetcher.FetchID(repoURL)
	}

	req, err := f.RequestCreator.CreateFetchRequest(repoURL, 0)
	if err != nil {
		return nil, err
	}

	fetcher, ok := f.RemoteFetchers[req.Endpoint.Version]
	if !ok {
		return nil, errors.New("repository_fetcher: fetching an image id: incompatible endpoint version")
	}

	return fetcher.FetchID(req)
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

func FetchError(context, registry, reponame string, err error) error {
	return garden.NewServiceUnavailableError(fmt.Sprintf("repository_fetcher: %s: could not fetch image %s from registry %s: %s", context, reponame, registry, err))
}
