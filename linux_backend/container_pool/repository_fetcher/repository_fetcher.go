package repository_fetcher

import (
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/dotcloud/docker/archive"
	"github.com/dotcloud/docker/image"
	"github.com/dotcloud/docker/registry"
)

type RepositoryFetcher interface {
	Fetch(repoName string, tag string) (imageID string, err error)
}

// apes docker's *registry.Registry
type Registry interface {
	GetRepositoryData(repoName string) (*registry.RepositoryData, error)
	GetRemoteTags(registries []string, repository string, token []string) (map[string]string, error)
	GetRemoteHistory(imageID string, registry string, token []string) ([]string, error)

	GetRemoteImageJSON(imageID string, registry string, token []string) ([]byte, int, error)
	GetRemoteImageLayer(imageID string, registry string, token []string, size int64) (io.ReadCloser, error)
}

// apes docker's *graph.Graph
type Graph interface {
	Exists(imageID string) bool
	Register(imageJSON []byte, layer archive.ArchiveReader, image *image.Image) error
}

type DockerRepositoryFetcher struct {
	registry Registry
	graph    Graph

	fetchingLayers map[string]chan struct{}
	fetchingMutex  *sync.Mutex
}

func New(registry Registry, graph Graph) RepositoryFetcher {
	return &DockerRepositoryFetcher{
		registry: registry,
		graph:    graph,

		fetchingLayers: map[string]chan struct{}{},
		fetchingMutex:  new(sync.Mutex),
	}
}

func (fetcher *DockerRepositoryFetcher) Fetch(repoName string, tag string) (string, error) {
	log.Println("fetching", repoName+":"+tag)

	repoData, err := fetcher.registry.GetRepositoryData(repoName)
	if err != nil {
		return "", err
	}

	tagsList, err := fetcher.registry.GetRemoteTags(repoData.Endpoints, repoName, repoData.Tokens)
	if err != nil {
		return "", err
	}

	imgID, ok := tagsList[tag]
	if !ok {
		return "", fmt.Errorf("unknown tag: %s:%s", repoName, tag)
	}

	token := repoData.Tokens

	for _, endpoint := range repoData.Endpoints {
		log.Println("trying endpoint", endpoint, "for", imgID)
		err = fetcher.fetchFromEndpoint(endpoint, imgID, token)
		if err == nil {
			return imgID, nil
		}
	}

	return "", fmt.Errorf("all endpoints failed: %s", err)
}

func (fetcher *DockerRepositoryFetcher) fetchFromEndpoint(endpoint string, imgID string, token []string) error {
	history, err := fetcher.registry.GetRemoteHistory(imgID, endpoint, token)
	if err != nil {
		return err
	}

	for i := len(history) - 1; i >= 0; i-- {
		err := fetcher.fetchLayer(endpoint, history[i], token)
		if err != nil {
			return err
		}
	}

	return nil
}

func (fetcher *DockerRepositoryFetcher) fetchLayer(endpoint string, layerID string, token []string) error {
	for acquired := false; !acquired; acquired = fetcher.fetching(layerID) {
	}

	defer fetcher.doneFetching(layerID)

	if fetcher.graph.Exists(layerID) {
		log.Println("already exists:", layerID)
		return nil
	}

	imgJSON, imgSize, err := fetcher.registry.GetRemoteImageJSON(layerID, endpoint, token)
	if err != nil {
		return err
	}

	img, err := image.NewImgJSON(imgJSON)
	if err != nil {
		return err
	}

	layer, err := fetcher.registry.GetRemoteImageLayer(img.ID, endpoint, token, int64(imgSize))
	if err != nil {
		return err
	}

	defer layer.Close()

	log.Println("downloading layer:", layerID)

	err = fetcher.graph.Register(imgJSON, layer, img)
	if err != nil {
		return err
	}

	log.Println("finished downloading:", layerID)

	return nil
}

func (fetcher *DockerRepositoryFetcher) fetching(layerID string) bool {
	fetcher.fetchingMutex.Lock()

	fetching, found := fetcher.fetchingLayers[layerID]
	if !found {
		fetcher.fetchingLayers[layerID] = make(chan struct{})
		fetcher.fetchingMutex.Unlock()
		return true
	} else {
		fetcher.fetchingMutex.Unlock()
		<-fetching
		return false
	}
}

func (fetcher *DockerRepositoryFetcher) doneFetching(layerID string) {
	fetcher.fetchingMutex.Lock()
	close(fetcher.fetchingLayers[layerID])
	delete(fetcher.fetchingLayers, layerID)
	fetcher.fetchingMutex.Unlock()
}
