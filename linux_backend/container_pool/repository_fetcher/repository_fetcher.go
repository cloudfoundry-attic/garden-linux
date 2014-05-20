package repository_fetcher

import (
	"fmt"
	"io"
	"log"

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
	GetRemoteImageLayer(imageID string, registry string, token []string) (io.ReadCloser, error)
}

// apes docker's *graph.Graph
type Graph interface {
	Exists(imageID string) bool
	Register(imageJSON []byte, layer archive.ArchiveReader, image *image.Image) error
}

type DockerRepositoryFetcher struct {
	registry Registry
	graph    Graph
}

func New(registry Registry, graph Graph) RepositoryFetcher {
	return &DockerRepositoryFetcher{
		registry: registry,
		graph:    graph,
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
		id := history[i]

		if fetcher.graph.Exists(id) {
			log.Println("already exists:", id)
			continue
		}

		imgJSON, _, err := fetcher.registry.GetRemoteImageJSON(id, endpoint, token)
		if err != nil {
			return err
		}

		img, err := image.NewImgJSON(imgJSON)
		if err != nil {
			return err
		}

		layer, err := fetcher.registry.GetRemoteImageLayer(img.ID, endpoint, token)
		if err != nil {
			return err
		}

		defer layer.Close()

		log.Println("downloading layer:", id)

		err = fetcher.graph.Register(imgJSON, layer, img)
		if err != nil {
			return err
		}
	}

	return nil
}
