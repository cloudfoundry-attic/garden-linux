package repository_fetcher

import (
	"fmt"
	"net/url"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
)

type ImageIDProvider struct {
	Providers map[string]ContainerIDProvider
}

func (provider *ImageIDProvider) ProvideID(path string) (layercake.ID, error) {
	rootfsURL, err := url.Parse(path)
	if err != nil {
		return nil, err
	}

	containerProvider, ok := provider.Providers[rootfsURL.Scheme]
	if !ok {
		return nil, fmt.Errorf("IDProvider could not be found for %s", path)
	}
	return containerProvider.ProvideID(path), nil
}
