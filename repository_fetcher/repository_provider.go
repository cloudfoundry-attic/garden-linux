package repository_fetcher

import (
	"fmt"
	"strings"

	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/pkg/transport"
	"github.com/docker/docker/registry"
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
	hostname = rp.ApplyDefaultHostname(hostname)

	endpoint, err := RegistryNewEndpoint(&registry.IndexInfo{
		Name:   hostname,
		Secure: !contains(rp.InsecureRegistries, hostname),
	}, nil)

	if err != nil && strings.Contains(err.Error(), "--insecure-registry") {
		return nil, &InsecureRegistryError{
			Cause:              err,
			Endpoint:           hostname,
			InsecureRegistries: rp.InsecureRegistries,
		}
	} else if err != nil {
		return nil, err
	}

	tr := transport.NewTransport(
		registry.NewTransport(registry.ReceiveTimeout, endpoint.IsSecure),
	)

	return RegistryNewSession(registry.HTTPClient(tr), &cliconfig.AuthConfig{}, endpoint)
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
