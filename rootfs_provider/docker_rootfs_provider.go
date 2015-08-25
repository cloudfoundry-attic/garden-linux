package rootfs_provider

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"sync"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/graph"
	"github.com/docker/docker/image"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
)

type dockerRootFSProvider struct {
	name          string
	graph         *graph.Graph
	graphDriver   graphdriver.Driver
	volumeCreator VolumeCreator
	repoFetcher   repository_fetcher.RepositoryFetcher
	namespacer    Namespacer
	clock         clock.Clock
	mutex         *sync.Mutex

	fallback RootFSProvider
}

//go:generate counterfeiter -o fake_graph_driver/fake_graph_driver.go . GraphDriver
type GraphDriver interface {
	graphdriver.Driver
}

func NewDocker(
	name string,
	repoFetcher repository_fetcher.RepositoryFetcher,
	graph *graph.Graph,
	graphDriver graphdriver.Driver,
	volumeCreator VolumeCreator,
	namespacer Namespacer,
	clock clock.Clock,
) (RootFSProvider, error) {
	return &dockerRootFSProvider{
		name:          name,
		repoFetcher:   repoFetcher,
		graph:         graph,
		graphDriver:   graphDriver,
		volumeCreator: volumeCreator,
		namespacer:    namespacer,
		clock:         clock,
		mutex:         &sync.Mutex{},
	}, nil
}

func (provider *dockerRootFSProvider) Name() string {
	return provider.name
}

func (provider *dockerRootFSProvider) ProvideRootFS(logger lager.Logger, id string, url *url.URL, shouldNamespace bool, quota int64) (string, process.Env, error) {
	tag := "latest"
	if len(url.Fragment) > 0 {
		tag = url.Fragment
	}

	imageID, envvars, volumes, err := provider.repoFetcher.Fetch(logger, url, tag, quota)
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

	id = fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
	err = provider.graph.Register(&image.Image{
		ID:     id,
		Parent: imageID,
	}, nil)
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
	namespacedImageID := imageID + "@" + provider.namespacer.CacheKey()
	namespacedImageID = fmt.Sprintf("%x", sha256.Sum256([]byte(namespacedImageID)))
	if !provider.graphDriver.Exists(namespacedImageID) {
		if err := provider.createNamespacedLayer(namespacedImageID, imageID); err != nil {
			return "", err
		}
	}

	return namespacedImageID, nil
}

func (provider *dockerRootFSProvider) createNamespacedLayer(id string, parentId string) error {
	var err error
	var path string
	if path, err = provider.createLayer(id, parentId); err != nil {
		return err
	}

	return provider.namespacer.Namespace(path)
}

func (provider *dockerRootFSProvider) createLayer(id, parentId string) (string, error) {
	errs := func(err error) (string, error) {
		return "", err
	}

	if err := provider.graph.Register(&image.Image{
		ID:     id,
		Parent: parentId,
	}, nil); err != nil {
		return errs(err)
	}

	namespacedRootfs, err := provider.graphDriver.Get(id, "")
	if err != nil {
		return errs(err)
	}

	return namespacedRootfs, nil
}
