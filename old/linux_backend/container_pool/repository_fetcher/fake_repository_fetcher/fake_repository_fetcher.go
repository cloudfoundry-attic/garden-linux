package fake_repository_fetcher

import (
	"sync"

	"github.com/pivotal-golang/lager"
)

type FakeRepositoryFetcher struct {
	fetched     []FetchSpec
	FetchResult string
	FetchError  error

	mutex *sync.RWMutex
}

type FetchSpec struct {
	Repository string
	Tag        string
}

func New() *FakeRepositoryFetcher {
	return &FakeRepositoryFetcher{
		mutex: &sync.RWMutex{},
	}
}

func (fetcher *FakeRepositoryFetcher) Fetch(logger lager.Logger, repoName string, tag string) (string, []string, error) {
	if fetcher.FetchError != nil {
		return "", nil, fetcher.FetchError
	}

	fetcher.mutex.Lock()
	fetcher.fetched = append(fetcher.fetched, FetchSpec{repoName, tag})
	fetcher.mutex.Unlock()
	envvars := []string{"env1", "env1Value", "env2", "env2Value"}
	return fetcher.FetchResult, envvars, nil
}

func (fetcher *FakeRepositoryFetcher) Fetched() []FetchSpec {
	fetcher.mutex.RLock()
	defer fetcher.mutex.RUnlock()

	return fetcher.fetched
}
