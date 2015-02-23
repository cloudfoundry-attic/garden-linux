package repository_fetcher

//go:generate counterfeiter . RegistryProvider
type RegistryProvider interface {
	ProvideRegistry(hostname string) (Registry, error)
}

type registryProvider struct {
	DefaultHostname string
}

func (rp registryProvider) ProvideRegistry(hostname string) (Registry, error) {
	var err error

	if hostname == "" {
		hostname = rp.DefaultHostname
	}

	endpoint, err := RegistryNewEndpoint(hostname, nil)
	if err != nil {
		return nil, err
	}

	return RegistryNewSession(nil, nil, endpoint, true)
}

func NewRepositoryProvider(defaultHostname string) RegistryProvider {
	return &registryProvider{DefaultHostname: defaultHostname}
}
