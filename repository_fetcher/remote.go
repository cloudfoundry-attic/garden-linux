package repository_fetcher

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"encoding/json"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/docker/distribution/digest"
	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
)

type RepositoryFetcher interface {
	Fetch(logger lager.Logger, url *url.URL, tag string) (imageID string, envvars process.Env, volumes []string, err error)
}

var ErrInvalidDockerURL = errors.New("invalid docker url")

// apes dockers registry.NewEndpoint
var RegistryNewEndpoint = registry.NewEndpoint

// apes dockers registry.NewSession
var RegistryNewSession = registry.NewSession

// apes docker's *registry.Registry
type Registry interface {
	// v1 methods
	GetRepositoryData(repoName string) (*registry.RepositoryData, error)
	GetRemoteTags(registries []string, repository string) (map[string]string, error)
	GetRemoteHistory(imageID string, registry string) ([]string, error)
	GetRemoteImageJSON(imageID string, registry string) ([]byte, int, error)
	GetRemoteImageLayer(imageID string, registry string, size int64) (io.ReadCloser, error)

	// v2 methods
	GetV2ImageManifest(ep *registry.Endpoint, imageName, tagName string, auth *registry.RequestAuthorization) (digest.Digest, []byte, error)
	GetV2ImageBlobReader(ep *registry.Endpoint, imageName string, dgst digest.Digest, auth *registry.RequestAuthorization) (io.ReadCloser, int64, error)
}

// apes docker's *graph.Graph
type Graph interface {
	Get(name string) (*image.Image, error)
	Exists(imageID string) bool
	Register(image *image.Image, layer archive.ArchiveReader) error
}

type DockerRepositoryFetcher struct {
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
	return &DockerRepositoryFetcher{
		registryProvider: registry,
		graph:            graph,
		fetchingLayers:   map[string]chan struct{}{},
		fetchingMutex:    new(sync.Mutex),
	}
}

func fetchError(context, registry, reponame string, err error) error {
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
		return errs(fetchError("ProvideRegistry", hostname, path, err))
	}

	if endpoint.Version == registry.APIVersion2 {
		auth := registry.NewRequestAuthorization(&cliconfig.AuthConfig{}, endpoint, "", "", []string{})
		_, manifestBytes, err := r.GetV2ImageManifest(endpoint, path, tag, auth)
		if err != nil {
			return errs(fetchError("GetV2ImageManifest", hostname, path, err))
		}

		var manifest registry.ManifestData
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return errs(fetchError("UnmarshalManifest", hostname, path, err))
		}

		var imageID string

		for i := len(manifest.FSLayers) - 1; i >= 0; i-- {
			hash, err := digest.ParseDigest(manifest.FSLayers[i].BlobSum)
			if err != nil {
				return errs(fetchError("ParseDigest", hostname, path, err))
			}

			img, err := image.NewImgJSON([]byte(manifest.History[i].V1Compatibility))
			if err != nil {
				return errs(fetchError("NewImgJSON", hostname, path, err))
			}
			if i == 0 {
				imageID = img.ID
			}

			if !fetcher.graph.Exists(img.ID) {
				reader, _, err := r.GetV2ImageBlobReader(endpoint, path, hash, auth)
				if err != nil {
					return errs(fetchError("GetV2ImageBlobReader", hostname, path, err))
				}
				defer reader.Close()

				err = fetcher.graph.Register(img, reader)
				if err != nil {
					return errs(fetchError("GraphRegister", hostname, path, err))
				}
			}
		}

		return imageID, process.Env{}, []string{}, nil
	} else if endpoint.Version == registry.APIVersion1 {
		repoData, err := r.GetRepositoryData(path)
		if err != nil {
			return errs(fetchError("GetRepositoryData", hostname, path, err))
		}

		tagsList, err := r.GetRemoteTags(repoData.Endpoints, path)
		if err != nil {
			return errs(fetchError("GetRemoteTags", hostname, path, err))
		}

		imgID, ok := tagsList[tag]
		if !ok {
			return errs(fetchError("looking up tag", hostname, path, fmt.Errorf("unknown tag: %v", tag)))
		}

		for _, endpoint := range repoData.Endpoints {
			fLog.Debug("trying", lager.Data{
				"endpoint": endpoint,
				"image":    imgID,
			})

			var image *dockerImage
			image, err = fetcher.fetchFromEndpoint(fLog, r, endpoint, imgID)
			if err == nil {
				fLog.Debug("fetched", lager.Data{
					"endpoint": endpoint,
					"image":    imgID,
					"env":      image.Env(),
					"volumes":  image.Vols(),
				})

				return imgID, image.Env(), image.Vols(), nil
			}
		}

		return errs(fetchError("fetchFromEndPoint", hostname, path, fmt.Errorf("all endpoints failed: %v", err)))
	}

	return errs(errors.New("Unknown docker registry API version"))
}

func (fetcher *DockerRepositoryFetcher) fetchFromEndpoint(logger lager.Logger, registry Registry, endpoint string, imgID string) (*dockerImage, error) {
	history, err := registry.GetRemoteHistory(imgID, endpoint)
	if err != nil {
		return nil, err
	}

	var allLayers []*dockerLayer
	for i := len(history) - 1; i >= 0; i-- {
		layer, err := fetcher.fetchLayer(logger, registry, endpoint, history[i])
		if err != nil {
			return nil, err
		}

		allLayers = append(allLayers, layer)
	}

	return &dockerImage{allLayers}, nil
}

func (fetcher *DockerRepositoryFetcher) fetchLayer(logger lager.Logger, registry Registry, endpoint string, layerID string) (*dockerLayer, error) {
	for acquired := false; !acquired; acquired = fetcher.fetching(layerID) {
	}

	defer fetcher.doneFetching(layerID)

	img, err := fetcher.graph.Get(layerID)
	if err == nil {
		logger.Info("using-cached", lager.Data{
			"layer": layerID,
		})

		return &dockerLayer{imgEnv(img, logger), imgVolumes(img)}, nil
	}

	imgJSON, imgSize, err := registry.GetRemoteImageJSON(layerID, endpoint)
	if err != nil {
		return nil, fmt.Errorf("get remote image JSON: %v", err)
	}

	img, err = image.NewImgJSON(imgJSON)
	if err != nil {
		return nil, fmt.Errorf("new image JSON: %v", err)
	}

	layer, err := registry.GetRemoteImageLayer(img.ID, endpoint, int64(imgSize))
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

	return &dockerLayer{imgEnv(img, logger), imgVolumes(img)}, nil
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

func imgEnv(img *image.Image, logger lager.Logger) process.Env {
	if img.Config == nil {
		return process.Env{}
	}

	return filterEnv(img.Config.Env, logger)
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

func filterEnv(env []string, logger lager.Logger) process.Env {
	var filtered []string
	for _, e := range env {
		segs := strings.SplitN(e, "=", 2)
		if len(segs) != 2 {
			// malformed docker image metadata?
			logger.Info("Unrecognised environment variable", lager.Data{"e": e})
			continue
		}
		filtered = append(filtered, e)
	}

	filteredWithNoDups, err := process.NewEnv(filtered)
	if err != nil {
		logger.Error("Invalid environment", err)
	}
	return filteredWithNoDups
}
