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

type DockerRepositoryFetcher struct {
	v1 *RemoteV1Fetcher
	v2 *RemoteV2Fetcher

	registryProvider RegistryProvider
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

func NewRemote(registry RegistryProvider, graph Graph) RepositoryFetcher {
	lock := &GraphLock{}

	return &DockerRepositoryFetcher{
		v1:               &RemoteV1Fetcher{Graph: graph, GraphLock: lock},
		v2:               &RemoteV2Fetcher{Graph: graph, GraphLock: lock},
		registryProvider: registry,
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
	hostname := fetcher.registryProvider.ApplyDefaultHostname(repoURL.Host)

	r, endpoint, err := fetcher.registryProvider.ProvideRegistry(hostname)
	if err != nil {
		logger.Error("failed-to-construct-registry-endpoint", err)
		return errs(FetchError("ProvideRegistry", hostname, path, err))
	}

	fetchRequest := &FetchRequest{
		Session:  r,
		Endpoint: endpoint,
		Logger:   fLog,
		Hostname: hostname,
		Path:     path,
		Tag:      tag,
	}

	var response *FetchResponse

	if endpoint.Version == registry.APIVersion2 {
		response, err = fetcher.v2.Fetch(fetchRequest)
		if err != nil {
			return errs(err)
		}
	} else if endpoint.Version == registry.APIVersion1 {
		response, err = fetcher.v1.Fetch(fetchRequest)
		if err != nil {
			return errs(err)
		}
	} else {
		return errs(errors.New("Unknown docker registry API version"))
	}

	return response.ImageID, response.Env, response.Volumes, nil
}

//func (fetcher *DockerRepositoryFetcher) fetching(layerID string) bool {
//	fetcher.fetchingMutex.Lock()
//
//	fetching, found := fetcher.fetchingLayers[layerID]
//	if !found {
//		fetcher.fetchingLayers[layerID] = make(chan struct{})
//		fetcher.fetchingMutex.Unlock()
//		return true
//	} else {
//		fetcher.fetchingMutex.Unlock()
//		<-fetching
//		return false
//	}
//}
//
//func (fetcher *DockerRepositoryFetcher) doneFetching(layerID string) {
//	fetcher.fetchingMutex.Lock()
//	close(fetcher.fetchingLayers[layerID])
//	delete(fetcher.fetchingLayers, layerID)
//	fetcher.fetchingMutex.Unlock()
//}
