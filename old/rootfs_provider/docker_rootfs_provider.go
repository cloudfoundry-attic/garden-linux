package rootfs_provider

import (
	"errors"
	"net/url"
	"sync"
	"syscall"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"

	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/old/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

type dockerRootFSProvider struct {
	graphDriver   graphdriver.Driver
	volumeCreator VolumeCreator
	repoFetcher   repository_fetcher.RepositoryFetcher
	namespacer    Namespacer
	clock         clock.Clock
	mutex         *sync.Mutex
	graphPath     string

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
	namespacer Namespacer,
	graphPath string,
	clock clock.Clock,
) (RootFSProvider, error) {
	return &dockerRootFSProvider{
		repoFetcher:   repoFetcher,
		graphDriver:   graphDriver,
		volumeCreator: volumeCreator,
		namespacer:    namespacer,
		graphPath:     graphPath,
		clock:         clock,
		mutex:         &sync.Mutex{},
	}, nil
}

func (provider *dockerRootFSProvider) ProvideRootFS(logger lager.Logger, id string, url *url.URL, shouldNamespace bool) (string, process.Env, error) {
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

	if shouldNamespace {
		provider.mutex.Lock()
		imageID, err = provider.namespace(imageID)
		provider.mutex.Unlock()
		if err != nil {
			return "", nil, err
		}
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

func (provider *dockerRootFSProvider) namespace(imageID string) (string, error) {
	namespacedImageID := imageID + "@namespaced"
	if !provider.graphDriver.Exists(namespacedImageID) {
		if err := provider.createNamespacedLayer(namespacedImageID, imageID); err != nil {
			return "", err
		}
	}

	return namespacedImageID, nil
}

func (provider *dockerRootFSProvider) createNamespacedLayer(id string, parentId string) error {
	path, err := provider.createLayer(id, parentId)
	if err != nil {
		return err
	}

	return provider.namespacer.Namespace(path)
}

func (provider *dockerRootFSProvider) createLayer(id, parentId string) (string, error) {
	errs := func(err error) (string, error) {
		return "", err
	}

	if err := provider.graphDriver.Create(id, parentId); err != nil {
		return errs(err)
	}

	namespacedRootfs, err := provider.graphDriver.Get(id, parentId)
	if err != nil {
		return errs(err)
	}

	return namespacedRootfs, nil
}

func (provider *dockerRootFSProvider) CleanupRootFS(logger lager.Logger, id string) error {
	provider.graphDriver.Put(id)
	if err := syscall.Unmount(filepath.Join(provider.graphPath, "overlayfs", id, "merged"), 0); err != nil {
		return err
	}

	err := provider.graphDriver.Remove(id)
	logger.Error("cleanup-rootfs", err)
	return err
}
