package fake_repository_fetcher

import (
	"net/url"
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
)

type FakeRepositoryFetcher struct {
	fetched []FetchSpec

	FetchResult   string
	FetchError    error
	FetchedLayers []string

	mutex *sync.RWMutex
}

type FetchSpec struct {
	URL       *url.URL
	DiskQuota int64
}

func New() *FakeRepositoryFetcher {
	return &FakeRepositoryFetcher{
		mutex: &sync.RWMutex{},
	}
}

func (fetcher *FakeRepositoryFetcher) Fetch(imageUrl *url.URL, quota int64) (*repository_fetcher.Image, error) {
	if fetcher.FetchError != nil {
		return nil, fetcher.FetchError
	}

	fetcher.mutex.Lock()
	fetcher.fetched = append(fetcher.fetched, FetchSpec{imageUrl, quota})
	fetcher.mutex.Unlock()
	envvars := process.Env{"env1": "env1Value", "env2": "env2Value"}
	volumes := []string{"/foo", "/bar"}

	id := fetcher.FetchResult
	return &repository_fetcher.Image{
		id, envvars, volumes, fetcher.FetchedLayers,
	}, nil
}

func (fetcher *FakeRepositoryFetcher) Fetched() []FetchSpec {
	fetcher.mutex.RLock()
	defer fetcher.mutex.RUnlock()

	return fetcher.fetched
}
