// The network fence provides Garden's networking function.
package network

import (
	"flag"

	"github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/cloudfoundry-incubator/garden-linux/fences/network/subnets"
)

const (
	DefaultNetworkPool        = "10.254.0.0/22"
	DefaultMTUSize     MtuVar = 1500
)

type Config struct {
	Network CidrVar
	Mtu     MtuVar
}

func init() {
	config := &Config{}
	fences.Register(config.Init, config.Main)
}

func (config *Config) Init(fs *flag.FlagSet) error {
	config.Network = cidrVar(DefaultNetworkPool)
	config.Mtu = DefaultMTUSize

	fs.Var(&config.Network, "networkPool",
		"Pool of dynamically allocated container subnets")

	fs.Var(&config.Mtu, "mtu",
		"MTU size for container network interfaces")

	return nil
}

func (config *Config) Main(registry *fences.BuilderRegistry) error {
	subnets, err := subnets.New(config.Network.IPNet)
	if err != nil {
		return err
	}

	fence := &f{subnets, uint32(config.Mtu)}
	registry.Register(fence)

	return nil
}
