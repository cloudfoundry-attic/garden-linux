package sysconfig

import "fmt"

type Config struct {
	CgroupPath             string
	VolumesPath            string
	NetworkInterfacePrefix string
	IPTables               IPTablesConfig
}

type IPTablesConfig struct {
	Filter IPTablesFilterConfig
	NAT    IPTablesNATConfig
}

type IPTablesFilterConfig struct {
	ForwardChain   string
	DefaultChain   string
	InstancePrefix string
}

type IPTablesNATConfig struct {
	PreroutingChain  string
	PostroutingChain string
	InstancePrefix   string
}

func NewConfig(tag string) Config {
	return Config{
		NetworkInterfacePrefix: fmt.Sprintf("w%s", tag),

		CgroupPath: fmt.Sprintf("/tmp/garden-%s/cgroup", tag),

		IPTables: IPTablesConfig{
			Filter: IPTablesFilterConfig{
				ForwardChain:   fmt.Sprintf("w-%s-forward", tag),
				DefaultChain:   fmt.Sprintf("w-%s-default", tag),
				InstancePrefix: fmt.Sprintf("w-%s-instance-", tag),
			},
			NAT: IPTablesNATConfig{
				PreroutingChain:  fmt.Sprintf("w-%s-prerouting", tag),
				PostroutingChain: fmt.Sprintf("w-%s-postrouting", tag),
				InstancePrefix:   fmt.Sprintf("w-%s-instance-", tag),
			},
		},
	}
}

func (config Config) Environ() []string {
	return []string{
		"GARDEN_CGROUP_PATH=" + config.CgroupPath,

		"GARDEN_NETWORK_INTERFACE_PREFIX=" + config.NetworkInterfacePrefix,

		"GARDEN_IPTABLES_FILTER_FORWARD_CHAIN=" + config.IPTables.Filter.ForwardChain,
		"GARDEN_IPTABLES_FILTER_DEFAULT_CHAIN=" + config.IPTables.Filter.DefaultChain,
		"GARDEN_IPTABLES_FILTER_INSTANCE_PREFIX=" + config.IPTables.Filter.InstancePrefix,

		"GARDEN_IPTABLES_NAT_PREROUTING_CHAIN=" + config.IPTables.NAT.PreroutingChain,
		"GARDEN_IPTABLES_NAT_POSTROUTING_CHAIN=" + config.IPTables.NAT.PostroutingChain,
		"GARDEN_IPTABLES_NAT_INSTANCE_PREFIX=" + config.IPTables.NAT.InstancePrefix,
	}
}
