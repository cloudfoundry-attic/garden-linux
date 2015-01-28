package rootfs_provider

import (
	"errors"
	"net/url"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

type dockerRootFSProvider struct {
	newRepoFetcher      func(registryName string) (repository_fetcher.RepositoryFetcher, error)
	defaultRegistryName string
	graphDriver         graphdriver.Driver
	volumeCreator       VolumeCreator

	fallback           RootFSProvider
	defaultRepoFetcher repository_fetcher.RepositoryFetcher
}

var ErrInvalidDockerURL = errors.New("invalid docker url")

func NewDocker(
	newRepoFetcher func(registryName string) (repository_fetcher.RepositoryFetcher, error),
	defaultRegistryName string,
	graphDriver graphdriver.Driver,
	volumeCreator VolumeCreator,
) (RootFSProvider, error) {
	defaultRepoFetcher, err := newRepoFetcher(defaultRegistryName)
	if err != nil {
		return nil, err
	}

	return &dockerRootFSProvider{
		newRepoFetcher:      newRepoFetcher,
		defaultRegistryName: defaultRegistryName,
		graphDriver:         graphDriver,
		volumeCreator:       volumeCreator,
		defaultRepoFetcher:  defaultRepoFetcher,
	}, nil
}

func (provider *dockerRootFSProvider) ProvideRootFS(logger lager.Logger, id string, url *url.URL) (string, process.Env, error) {
	repoFetcher := provider.defaultRepoFetcher
	if url.Host != "" {
		var err error
		repoFetcher, err = provider.newRepoFetcher(url.Host)
		if err != nil {
			logger.Error("failed-to-create-repository-fetcher", err, lager.Data{"url": url})
			return "", nil, ErrInvalidDockerURL
		}
	}

	if len(url.Path) == 0 {
		return "", nil, ErrInvalidDockerURL
	}

	repoName := url.Path[1:]

	tag := "latest"
	if len(url.Fragment) > 0 {
		tag = url.Fragment
	}

	imageID, envvars, volumes, err := repoFetcher.Fetch(logger, repoName, tag)
	if err != nil {
		return "", nil, err
	}

	err = provider.graphDriver.Create(id, imageID)
	if err != nil {
		return "", nil, err
	}

	rootPath, err := provider.graphDriver.Get(id, "")
	if err != nil {
		return "", nil, err
	}

	for _, v := range volumes {
		if err = provider.volumeCreator.Create(rootPath, v); err != nil {
			return "", nil, err
		}
	}

	return rootPath, envvars, nil
}

func (provider *dockerRootFSProvider) CleanupRootFS(logger lager.Logger, id string) error {
	provider.graphDriver.Put(id)

	return provider.graphDriver.Remove(id)
}
