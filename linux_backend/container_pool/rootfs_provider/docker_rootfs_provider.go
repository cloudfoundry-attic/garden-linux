package rootfs_provider

import (
	"errors"
	"net/url"

	"github.com/docker/docker/daemon/graphdriver"

	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/repository_fetcher"
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

func (provider *dockerRootFSProvider) ProvideRootFS(id string, url *url.URL) (string, error) {
	if len(url.Path) == 0 {
		return "", ErrInvalidDockerURL
	}

	repoName := url.Path[1:]

	tag := "latest"
	if len(url.Fragment) > 0 {
		tag = url.Fragment
	}

	imageID, err := provider.repoFetcher.Fetch(repoName, tag)
	if err != nil {
		return "", err
	}

	err = provider.graphDriver.Create(id, imageID)
	if err != nil {
		return "", err
	}

	return provider.graphDriver.Get(id, "")
}

func (provider *dockerRootFSProvider) CleanupRootFS(id string) error {
	provider.graphDriver.Put(id)

	return provider.graphDriver.Remove(id)
}
