package fake_rootfs_provider

import (
	"net/url"
	"sync"

	"github.com/pivotal-golang/lager"
)

type FakeRootFSProvider struct {
	provided      []ProvidedSpec
	ProvideError  error
	ProvideResult string

	cleanedUp    []string
	CleanupError error

	mutex *sync.Mutex
}

type ProvidedSpec struct {
	ID  string
	URL *url.URL
}

func New() *FakeRootFSProvider {
	return &FakeRootFSProvider{
		mutex: new(sync.Mutex),
	}
}

func (provider *FakeRootFSProvider) ProvideRootFS(logger lager.Logger, id string, url *url.URL) (string, []string, error) {
	if provider.ProvideError != nil {
		return "", nil, provider.ProvideError
	}

	provider.mutex.Lock()
	provider.provided = append(provider.provided, ProvidedSpec{id, url})
	provider.mutex.Unlock()

	return provider.ProvideResult, nil, nil
}

func (provider *FakeRootFSProvider) CleanupRootFS(logger lager.Logger, id string) error {
	if provider.CleanupError != nil {
		return provider.CleanupError
	}

	provider.mutex.Lock()
	provider.cleanedUp = append(provider.cleanedUp, id)
	provider.mutex.Unlock()

	return nil
}

func (provider *FakeRootFSProvider) Provided() []ProvidedSpec {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()

	return provider.provided
}

func (provider *FakeRootFSProvider) CleanedUp() []string {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()

	return provider.cleanedUp
}
