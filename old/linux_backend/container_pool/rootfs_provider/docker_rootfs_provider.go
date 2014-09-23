package rootfs_provider

import (
	"errors"
	"net/url"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/repository_fetcher"
)

type dockerRootFSProvider struct {
	repoFetcher repository_fetcher.RepositoryFetcher
	graphDriver graphdriver.Driver

	fallback RootFSProvider
}

var ErrInvalidDockerURL = errors.New("invalid docker url; must provide path")

func NewDocker(
	repoFetcher repository_fetcher.RepositoryFetcher,
	graphDriver graphdriver.Driver,
) RootFSProvider {
	return &dockerRootFSProvider{
		repoFetcher: repoFetcher,
		graphDriver: graphDriver,
	}
}

func (provider *dockerRootFSProvider) ProvideRootFS(logger lager.Logger, id string, url *url.URL) (string, []string, error) {
	if len(url.Path) == 0 {
		return "", nil, ErrInvalidDockerURL
	}

	repoName := url.Path[1:]

	tag := "latest"
	if len(url.Fragment) > 0 {
		tag = url.Fragment
	}

	imageID, envvars, err := provider.repoFetcher.Fetch(logger, repoName, tag)
	if err != nil {
		return "", nil, err
	}

	err = provider.graphDriver.Create(id, imageID)
	if err != nil {
		return "", nil, err
	}

	rootID, err := provider.graphDriver.Get(id, "")
	if err != nil {
		return "", nil, err
	}

	return rootID, envvars, nil
}

func (provider *dockerRootFSProvider) CleanupRootFS(logger lager.Logger, id string) error {
	provider.graphDriver.Put(id)

	return provider.graphDriver.Remove(id)
}
