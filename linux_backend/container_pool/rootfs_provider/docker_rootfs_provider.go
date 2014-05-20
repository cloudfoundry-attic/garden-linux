package rootfs_provider

import (
	"strings"
	"sync"

	"github.com/dotcloud/docker/daemon/graphdriver"

	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/repository_fetcher"
)

const imagePrefix = "image:"

type dockerRootFSProvider struct {
	repoFetcher repository_fetcher.RepositoryFetcher
	graphDriver graphdriver.Driver

	providedAsImage      map[string]bool
	providedAsImageMutex *sync.RWMutex

	fallback RootFSProvider
}

func NewDocker(
	repoFetcher repository_fetcher.RepositoryFetcher,
	graphDriver graphdriver.Driver,
	fallback RootFSProvider,
) RootFSProvider {
	return &dockerRootFSProvider{
		repoFetcher: repoFetcher,
		graphDriver: graphDriver,
		fallback:    fallback,

		providedAsImage:      map[string]bool{},
		providedAsImageMutex: new(sync.RWMutex),
	}
}

func (provider *dockerRootFSProvider) ProvideRootFS(id, path string) (string, error) {
	if !strings.HasPrefix(path, imagePrefix) {
		mountpoint, err := provider.fallback.ProvideRootFS(id, path)
		if err != nil {
			return "", err
		}

		provider.markProvidedAsImage(id, false)

		return mountpoint, nil
	}

	provider.markProvidedAsImage(id, true)

	repoSegments := strings.SplitN(path[len(imagePrefix):], ":", 2)

	repoName := repoSegments[0]

	tag := "latest"
	if len(repoSegments) >= 2 {
		tag = repoSegments[1]
	}

	imageID, err := provider.repoFetcher.Fetch(repoName, tag)
	if err != nil {
		return "", err
	}

	err = provider.graphDriver.Create(id, imageID)
	if err != nil {
		return "", err
	}

	mountpoint, err := provider.graphDriver.Get(id, "")
	if err != nil {
		return "", err
	}

	provider.markProvidedAsImage(id, true)

	return mountpoint, nil
}

func (provider *dockerRootFSProvider) CleanupRootFS(id string) error {
	provider.providedAsImageMutex.RLock()
	asImage := provider.providedAsImage[id]
	provider.providedAsImageMutex.RUnlock()

	if asImage {
		provider.graphDriver.Put(id)

		return provider.graphDriver.Remove(id)
	} else {
		return provider.fallback.CleanupRootFS(id)
	}
}

func (provider *dockerRootFSProvider) markProvidedAsImage(id string, asImage bool) {
	provider.providedAsImageMutex.Lock()
	provider.providedAsImage[id] = asImage
	provider.providedAsImageMutex.Unlock()
}
