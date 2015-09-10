package repository_fetcher

import (
	"fmt"
	"net/url"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/docker/docker/registry"
)

//go:generate counterfeiter -o fake_remote_image_id_provider/FakeRemoteImageIDProvider.go . RemoteImageIDProvider
type RemoteImageIDProvider interface {
	ProvideImageID(request *FetchRequest) (layercake.ID, error)
}

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

type RemoteIDProvider struct {
	RequestCreator FetchRequestCreator
	Providers      map[registry.APIVersion]RemoteImageIDProvider
}

func (provider *RemoteIDProvider) ProvideID(rawURL string) (layercake.ID, error) {
	rootfsURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	request, err := provider.RequestCreator.CreateFetchRequest(nil, rootfsURL, rootfsURL.Fragment, 0)
	if err != nil {
		return nil, err
	}

	APIVersion := request.Endpoint.Version

	registryProvider, ok := provider.Providers[APIVersion]
	if !ok {
		return nil, fmt.Errorf("could not find registryProvider for %d", APIVersion)
	}

	imageID, err := registryProvider.ProvideImageID(request)
	if err != nil {
		return nil, err
	}

	return imageID, nil
}
