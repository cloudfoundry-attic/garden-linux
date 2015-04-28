package repository_fetcher

import (
	"fmt"
	"strings"
)

//go:generate counterfeiter . RegistryProvider
type RegistryProvider interface {
	ProvideRegistry(hostname string) (Registry, error)
	ApplyDefaultHostname(hostname string) string
}

type InsecureRegistryError struct {
	Cause              error
	Endpoint           string
	InsecureRegistries []string
}

func (err InsecureRegistryError) Error() string {
	return fmt.Sprintf(
		"Registry %s is missing from -insecureDockerRegistryList (%v)",
		err.Endpoint,
		err.InsecureRegistries,
	)
}

type registryProvider struct {
	DefaultHostname    string
	InsecureRegistries []string
}

func (rp registryProvider) ApplyDefaultHostname(hostname string) string {
	if hostname == "" {
		return rp.DefaultHostname
	}
	return hostname
}

func (rp registryProvider) ProvideRegistry(hostname string) (Registry, error) {
	var err error

	hostname = rp.ApplyDefaultHostname(hostname)

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

	registry, err := RegistryNewSession(nil, nil, endpoint, true)
	if err != nil {
		return nil, err
	}

	return registry, nil
}

func NewRepositoryProvider(defaultHostname string, insecureRegistries []string) RegistryProvider {
	return &registryProvider{DefaultHostname: defaultHostname, InsecureRegistries: insecureRegistries}
}
