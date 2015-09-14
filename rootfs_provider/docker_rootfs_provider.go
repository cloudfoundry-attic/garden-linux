package rootfs_provider

import (
	"net/url"
	"sync"

	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
)

type dockerRootFSProvider struct {
	name          string
	graph         Graph
	volumeCreator VolumeCreator
	repoFetcher   repository_fetcher.RepositoryFetcher
	namespacer    Namespacer
	clock         clock.Clock
	mutex         *sync.Mutex

	fallback RootFSProvider
}

func NewDocker(
	name string,
	repoFetcher repository_fetcher.RepositoryFetcher,
	graph Graph,
	volumeCreator VolumeCreator,
	namespacer Namespacer,
	clock clock.Clock,
) (RootFSProvider, error) {
	return &dockerRootFSProvider{
		name:          name,
		repoFetcher:   repoFetcher,
		graph:         graph,
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
	if len(url.Fragment) == 0 {
		url.Fragment = "latest"
	}

	fetchedID, envvars, volumes, err := provider.repoFetcher.Fetch(logger, url, quota)
	if err != nil {
		return "", nil, err
	}

	var imageID layercake.ID = layercake.DockerImageID(fetchedID)
	if shouldNamespace {
		provider.mutex.Lock()
		imageID, err = provider.namespace(imageID)
		provider.mutex.Unlock()
		if err != nil {
			return "", nil, err
		}
	}

	containerID := layercake.ContainerID(id)
	err = provider.graph.Create(containerID, imageID)
	if err != nil {
		return "", nil, err
	}

	rootPath, err := provider.graph.Path(containerID)
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

func (provider *dockerRootFSProvider) namespace(imageID layercake.ID) (layercake.ID, error) {
	namespacedImageID := layercake.NamespacedLayerID{imageID.GraphID(), provider.namespacer.CacheKey()}
	if _, err := provider.graph.Get(namespacedImageID); err != nil {
		if err := provider.createNamespacedLayer(namespacedImageID, imageID); err != nil {
			return nil, err
		}
	}

	return namespacedImageID, nil
}

func (provider *dockerRootFSProvider) createNamespacedLayer(id, parentId layercake.ID) error {
	var err error
	var path string
	if path, err = provider.createLayer(id, parentId); err != nil {
		return err
	}

	return provider.namespacer.Namespace(path)
}

func (provider *dockerRootFSProvider) createLayer(id, parentId layercake.ID) (string, error) {
	errs := func(err error) (string, error) {
		return "", err
	}

	if err := provider.graph.Create(id, parentId); err != nil {
		return errs(err)
	}

	namespacedRootfs, err := provider.graph.Path(id)
	if err != nil {
		return errs(err)
	}

	return namespacedRootfs, nil
}
