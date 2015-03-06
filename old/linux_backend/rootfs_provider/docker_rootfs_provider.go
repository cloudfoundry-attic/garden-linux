package rootfs_provider

import (
	"errors"
	"net/url"
	"time"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

type dockerRootFSProvider struct {
	graphDriver   graphdriver.Driver
	volumeCreator VolumeCreator
	repoFetcher   repository_fetcher.RepositoryFetcher
	clock         clock.Clock

	fallback RootFSProvider
}

var ErrInvalidDockerURL = errors.New("invalid docker url")

//go:generate counterfeiter -o fake_graph_driver/fake_graph_driver.go . GraphDriver
type GraphDriver interface {
	graphdriver.Driver
}

func NewDocker(
	repoFetcher repository_fetcher.RepositoryFetcher,
	graphDriver GraphDriver,
	volumeCreator VolumeCreator,
	clock clock.Clock,
) (RootFSProvider, error) {
	return &dockerRootFSProvider{
		repoFetcher:   repoFetcher,
		graphDriver:   graphDriver,
		volumeCreator: volumeCreator,
		clock:         clock,
	}, nil
}

func (provider *dockerRootFSProvider) ProvideRootFS(logger lager.Logger, id string, url *url.URL) (string, process.Env, error) {
	if len(url.Path) == 0 {
		return "", nil, ErrInvalidDockerURL
	}

	tag := "latest"
	if len(url.Fragment) > 0 {
		tag = url.Fragment
	}

	imageID, envvars, volumes, err := provider.repoFetcher.Fetch(logger, url, tag)
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

	var err error
	maxAttempts := 10

	for errorCount := 0; errorCount < maxAttempts; errorCount++ {
		err = provider.graphDriver.Remove(id)
		if err == nil {
			break
		}

		logger.Error("cleanup-rootfs", err, lager.Data{
			"current-attempts": errorCount + 1,
			"max-attempts":     maxAttempts,
		})

		provider.clock.Sleep(200 * time.Millisecond)
	}

	return err
}
