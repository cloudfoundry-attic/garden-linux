package repository_fetcher

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
)

type RepositoryFetcher interface {
	Fetch(logger lager.Logger, repoName string, tag string) (imageID string, envvars []string, volumes []string, err error)
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
	Register(image *image.Image, layer archive.ArchiveReader) error
}

type DockerRepositoryFetcher struct {
	registry Registry
	graph    Graph

	fetchingLayers map[string]chan struct{}
	fetchingMutex  *sync.Mutex
}

type dockerImage struct {
	layers []*dockerLayer
}

func (d dockerImage) Env() []string {
	var envs []string
	for _, l := range d.layers {
		envs = append(envs, l.env...)
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
	env  []string
	vols []string
}

func New(registry Registry, graph Graph) RepositoryFetcher {
	return &DockerRepositoryFetcher{
		registry:       registry,
		graph:          graph,
		fetchingLayers: map[string]chan struct{}{},
		fetchingMutex:  new(sync.Mutex),
	}
}

func (fetcher *DockerRepositoryFetcher) Fetch(logger lager.Logger, repoName string, tag string) (string, []string, []string, error) {
	fLog := logger.Session("fetch", lager.Data{
		"repo": repoName,
		"tag":  tag,
	})

	fLog.Debug("fetching")

	repoData, err := fetcher.registry.GetRepositoryData(repoName)
	if err != nil {
		return "", nil, nil, err
	}

	tagsList, err := fetcher.registry.GetRemoteTags(repoData.Endpoints, repoName, repoData.Tokens)
	if err != nil {
		return "", nil, nil, err
	}

	imgID, ok := tagsList[tag]
	if !ok {
		return "", nil, nil, fmt.Errorf("unknown tag: %s:%s", repoName, tag)
	}

	token := repoData.Tokens

	for _, endpoint := range repoData.Endpoints {
		fLog.Debug("trying", lager.Data{
			"endpoint": endpoint,
			"image":    imgID,
		})

		image, err := fetcher.fetchFromEndpoint(fLog, endpoint, imgID, token)
		if err == nil {
			fLog.Debug("fetched", lager.Data{
				"endpoint": endpoint,
				"image":    imgID,
				"env":      image.Env(),
				"volumes":  image.Vols(),
			})

			return imgID, filterEnv(image.Env(), logger), image.Vols(), nil
		}
	}

	return "", nil, nil, fmt.Errorf("all endpoints failed: %v", err)
}

func (fetcher *DockerRepositoryFetcher) fetchFromEndpoint(logger lager.Logger, endpoint string, imgID string, token []string) (*dockerImage, error) {
	history, err := fetcher.registry.GetRemoteHistory(imgID, endpoint, token)
	if err != nil {
		return nil, err
	}

	var allLayers []*dockerLayer
	for i := len(history) - 1; i >= 0; i-- {
		layer, err := fetcher.fetchLayer(logger, endpoint, history[i], token)
		if err != nil {
			return nil, err
		}

		allLayers = append(allLayers, layer)
	}

	return &dockerImage{allLayers}, nil
}

func (fetcher *DockerRepositoryFetcher) fetchLayer(logger lager.Logger, endpoint string, layerID string, token []string) (*dockerLayer, error) {
	for acquired := false; !acquired; acquired = fetcher.fetching(layerID) {
	}

	defer fetcher.doneFetching(layerID)

	img, err := fetcher.graph.Get(layerID)
	if err == nil {
		logger.Info("using-cached", lager.Data{
			"layer": layerID,
		})

		return &dockerLayer{imgEnv(img), imgVolumes(img)}, nil
	}

	imgJSON, imgSize, err := fetcher.registry.GetRemoteImageJSON(layerID, endpoint, token)
	if err != nil {
		return nil, fmt.Errorf("get remote image JSON: %v", err)
	}

	img, err = image.NewImgJSON(imgJSON)
	if err != nil {
		return nil, fmt.Errorf("new image JSON: %v", err)
	}

	layer, err := fetcher.registry.GetRemoteImageLayer(img.ID, endpoint, token, int64(imgSize))
	if err != nil {
		return nil, fmt.Errorf("get remote image layer: %v", err)
	}

	defer layer.Close()

	started := time.Now()

	logger.Info("downloading", lager.Data{
		"layer": layerID,
	})

	err = fetcher.graph.Register(img, layer)
	if err != nil {
		return nil, fmt.Errorf("register: %s", err)
	}

	logger.Info("downloaded", lager.Data{
		"layer": layerID,
		"took":  time.Since(started),
		"vols":  imgVolumes(img),
	})

	return &dockerLayer{imgEnv(img), imgVolumes(img)}, nil
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

func imgEnv(img *image.Image) []string {
	var env []string

	if img.Config != nil {
		env = img.Config.Env
	}

	return env
}

func imgVolumes(img *image.Image) []string {
	var volumes []string

	if img.Config != nil {
		for volumePath, _ := range img.Config.Volumes {
			volumes = append(volumes, volumePath)
		}
	}

	return volumes
}

// multiple layers may specify environment variables; they are collected with
// the deepest layer first, so the first occurrence of the variable should win
func filterEnv(env []string, logger lager.Logger) []string {
	seen := map[string]bool{}

	var filtered []string
	for _, e := range env {
		segs := strings.SplitN(e, "=", 2)
		if len(segs) != 2 {
			// malformed docker image metadata?
			logger.Info("Unrecognised environment variable", lager.Data{"e": e})
			continue
		}

		if seen[segs[0]] {
			continue
		}

		filtered = append(filtered, e)
		seen[segs[0]] = true
	}

	return filtered
}
