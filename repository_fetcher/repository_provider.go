package repository_fetcher

import (
	"fmt"
	"strings"

	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/pkg/transport"
	"github.com/docker/docker/registry"
)

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

// TODO test new signature!!
func (rp registryProvider) ProvideRegistry(hostname string) (*registry.Session, *registry.Endpoint, error) {
	hostname = rp.ApplyDefaultHostname(hostname)

	endpoint, err := RegistryNewEndpoint(&registry.IndexInfo{
		Name:   hostname,
		Secure: !contains(rp.InsecureRegistries, hostname),
	}, nil)

	if err != nil && strings.Contains(err.Error(), "--insecure-registry") {
		return nil, nil, &InsecureRegistryError{
			Cause:              err,
			Endpoint:           hostname,
			InsecureRegistries: rp.InsecureRegistries,
		}
	} else if err != nil {
		return nil, nil, err
	}

	tr := transport.NewTransport(
		registry.NewTransport(registry.ReceiveTimeout, endpoint.IsSecure),
	)

	r, err := RegistryNewSession(registry.HTTPClient(tr), &cliconfig.AuthConfig{}, endpoint)
	return r, endpoint, err
}

func NewRepositoryProvider(defaultHostname string, insecureRegistries []string) RegistryProvider {
	return &registryProvider{DefaultHostname: defaultHostname, InsecureRegistries: insecureRegistries}
}

// #DRY
func contains(list []string, element string) bool {
	for _, e := range list {
		if e == element {
			return true
		}
	}
	return false
}
