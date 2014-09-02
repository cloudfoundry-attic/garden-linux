package repository_fetcher

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/archive"
	"github.com/docker/docker/image"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
)

type RepositoryFetcher interface {
	Fetch(logger lager.Logger, repoName string, tag string) (imageID string, envvars []string, err error)
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
	Get(name string) (*image.Image, error)
	Exists(imageID string) bool
	Register(image *image.Image, imageJSON []byte, layer archive.ArchiveReader) error
}

type DockerRepositoryFetcher struct {
	registry Registry
	graph    Graph

	fetchingLayers map[string]chan struct{}
	fetchingMutex  *sync.Mutex
	envvars        map[string]string
}

func New(registry Registry, graph Graph) RepositoryFetcher {
	return &DockerRepositoryFetcher{
		registry: registry,
		graph:    graph,

		fetchingLayers: map[string]chan struct{}{},
		fetchingMutex:  new(sync.Mutex),
		envvars:        map[string]string{},
	}
}

func (fetcher *DockerRepositoryFetcher) Fetch(logger lager.Logger, repoName string, tag string) (string, []string, error) {
	fLog := logger.Session("fetch", lager.Data{
		"repo": repoName,
		"tag":  tag,
	})

	fLog.Debug("fetching")

	repoData, err := fetcher.registry.GetRepositoryData(repoName)
	if err != nil {
		return "", nil, err
	}

	tagsList, err := fetcher.registry.GetRemoteTags(repoData.Endpoints, repoName, repoData.Tokens)
	if err != nil {
		return "", nil, err
	}

	imgID, ok := tagsList[tag]
	if !ok {
		return "", nil, fmt.Errorf("unknown tag: %s:%s", repoName, tag)
	}

	token := repoData.Tokens

	for _, endpoint := range repoData.Endpoints {
		fLog.Debug("trying", lager.Data{
			"endpoint": endpoint,
			"image":    imgID,
		})

		err = fetcher.fetchFromEndpoint(fLog, endpoint, imgID, token)
		if err == nil {
			var envvars []string
			if len(fetcher.envvars) > 0 {
				envvars = make([]string, len(fetcher.envvars))
				index := 0
				for key, value := range fetcher.envvars {
					envvars[index] = key + "=" + value
					index = index + 1
				}
			}
			return imgID, envvars, nil
		}
	}

	return "", nil, fmt.Errorf("all endpoints failed: %s", err)
}

func (fetcher *DockerRepositoryFetcher) fetchFromEndpoint(logger lager.Logger, endpoint string, imgID string, token []string) error {
	history, err := fetcher.registry.GetRemoteHistory(imgID, endpoint, token)
	if err != nil {
		return err
	}

	for i := len(history) - 1; i >= 0; i-- {
		err := fetcher.fetchLayer(logger, endpoint, history[i], token)
		if err != nil {
			return err
		}
	}

	return nil
}

func (fetcher *DockerRepositoryFetcher) fetchLayer(logger lager.Logger, endpoint string, layerID string, token []string) error {
	for acquired := false; !acquired; acquired = fetcher.fetching(layerID) {
	}

	defer fetcher.doneFetching(layerID)

	img, err := fetcher.graph.Get(layerID)
	if err == nil {
		logger.Info("using-cached", lager.Data{
			"layer": layerID,
		})
		// pull env vars from local graph storage since we have the image layer
		fetcher.collectEnvVars(img)
		return nil
	}

	imgJSON, imgSize, err := fetcher.registry.GetRemoteImageJSON(layerID, endpoint, token)
	if err != nil {
		return err
	}

	img, err = image.NewImgJSON(imgJSON)
	if err != nil {
		return err
	}

	fetcher.collectEnvVars(img)

	layer, err := fetcher.registry.GetRemoteImageLayer(img.ID, endpoint, token, int64(imgSize))
	if err != nil {
		return err
	}

	defer layer.Close()

	started := time.Now()

	logger.Info("downloading", lager.Data{
		"layer": layerID,
	})

	err = fetcher.graph.Register(img, imgJSON, layer)
	if err != nil {
		return err
	}

	logger.Info("downloaded", lager.Data{
		"layer": layerID,
		"took":  time.Since(started),
	})

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

func (fetcher *DockerRepositoryFetcher) collectEnvVars(img *image.Image) {
	if img.Config != nil {
		//NOTE: We use a map for the env vars because they may appear in multiple layers, given
		//we are fetching layer from the top down (back in time), the first occurance for the env
		//name wins
		for _, env := range img.Config.Env {
			keyValue := strings.SplitN(env, "=", 2)
			_, containsKey := fetcher.envvars[keyValue[0]]
			if len(keyValue) == 2 && !containsKey {
				fetcher.envvars[keyValue[0]] = keyValue[1]
			}
		}
	}
}
