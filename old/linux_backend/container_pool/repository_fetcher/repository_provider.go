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
		"repository_provider: Unable to fetch RootFS image from docker://%s.  To enable insecure access from this host, add it to the -insecureDockerRegistryList on boot.",
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
		return nil, &InsecureRegistryError{
			Cause:              err,
			Endpoint:           hostname,
			InsecureRegistries: rp.InsecureRegistries,
		}
	} else if err != nil {
		return nil, err
	}

	return RegistryNewSession(nil, nil, endpoint, true)
}

func NewRepositoryProvider(defaultHostname string, insecureRegistries []string) RegistryProvider {
	return &registryProvider{DefaultHostname: defaultHostname, InsecureRegistries: insecureRegistries}
}
