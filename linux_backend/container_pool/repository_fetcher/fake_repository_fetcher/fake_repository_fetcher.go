package fake_repository_fetcher

import "sync"

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

func (fetcher *FakeRepositoryFetcher) Fetch(repoName string, tag string) (string, error) {
	if fetcher.FetchError != nil {
		return "", fetcher.FetchError
	}

	fetcher.mutex.Lock()
	fetcher.fetched = append(fetcher.fetched, FetchSpec{repoName, tag})
	fetcher.mutex.Unlock()

	return fetcher.FetchResult, nil
}

func (fetcher *FakeRepositoryFetcher) Fetched() []FetchSpec {
	fetcher.mutex.RLock()
	defer fetcher.mutex.RUnlock()

	return fetcher.fetched
}
