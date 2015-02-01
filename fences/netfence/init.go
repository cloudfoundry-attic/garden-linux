package netfence

import (
	"flag"
	"net"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/subnets"
	"github.com/cloudfoundry/gunk/localip"
)

const (
	DefaultNetworkPool        = "10.254.0.0/22"
	DefaultMTUSize     MtuVar = 1500
)

type Config struct {
	Network    CidrVar
	Mtu        MtuVar
	ExternalIP IPVar
}

var Tag string

func init() {
	config := &Config{}
	fences.Register(config.Init, config.Main)
}

func (config *Config) Init(fs *flag.FlagSet) error {
	localIP, err := localip.LocalIP()
	if err != nil {
		return err
	}

	config.Network = cidrVar(DefaultNetworkPool)
	config.Mtu = DefaultMTUSize
	config.ExternalIP = IPVar{net.ParseIP(localIP)}

	fs.Var(&config.Network, "networkPool",
		"Pool of dynamically allocated container subnets")

	fs.Var(&config.Mtu, "mtu",
		"MTU size for container network interfaces")

	fs.Var(&config.ExternalIP, "externalIP",
		"IP address to use to reach container's mapped ports")

	fs.StringVar(&Tag, "tag", "", "server-wide identifier used for 'global' configuration")

	return nil
}

func (config *Config) Main(registry *fences.BuilderRegistry) error {
	prefix := "w" + Tag
	subnets, err := subnets.NewBridgedSubnets(config.Network.IPNet, prefix)
	if err != nil {
		return err
	}

	log := cf_lager.New("netfence")
	fence := &fenceBuilder{
		BridgedSubnets: subnets,
		mtu:            uint32(config.Mtu),
		externalIP:     config.ExternalIP.IP,
		deconfigurer:   network.NewDeconfigurer(),
		log:            log,
	}
	registry.Register(fence)

	return nil
}
