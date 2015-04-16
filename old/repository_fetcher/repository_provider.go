package repository_fetcher

import (
	"fmt"
	"strings"
)

//go:generate counterfeiter . RegistryProvider
type RegistryProvider interface {
	ProvideRegistry(hostname string) (Registry, error)
}

type InsecureRegistryError struct {
	Cause              error
	Endpoint           string
	InsecureRegistries []string
}

func (err InsecureRegistryError) Error() string {
	return fmt.Sprintf(
		"Unable to fetch RootFS image: To enable insecure access from this host, add it to the -insecureDockerRegistryList on boot.",
		err.Endpoint,
	)
}

type registryProvider struct {
	DefaultHostname    string
	InsecureRegistries []string
}

func (rp registryProvider) ProvideRegistry(hostname string) (Registry, error) {
	var err error

	if hostname == "" {
		hostname = rp.DefaultHostname
	}

	endpoint, err := RegistryNewEndpoint(hostname, rp.InsecureRegistries)
	if err != nil && strings.Contains(err.Error(), "--insecure-registry") {
		return &RegistryStringer{nil, hostname}, &InsecureRegistryError{
			Cause:              err,
			Endpoint:           hostname,
			InsecureRegistries: rp.InsecureRegistries,
		}
	} else if err != nil {
		//return nil, err
		return &RegistryStringer{nil, hostname}, err
	}

	registry, err := RegistryNewSession(nil, nil, endpoint, true)

	return &RegistryStringer{registry, hostname}, err
}

func NewRepositoryProvider(defaultHostname string, insecureRegistries []string) RegistryProvider {
	return &registryProvider{DefaultHostname: defaultHostname, InsecureRegistries: insecureRegistries}
}
