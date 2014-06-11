package sysconfig

import "fmt"

type Config struct {
	CgroupPath             string
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

		CgroupPath: fmt.Sprintf("/tmp/warden-%s/cgroup", tag),

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
		"WARDEN_CGROUP_PATH=" + config.CgroupPath,

		"WARDEN_NETWORK_INTERFACE_PREFIX=" + config.NetworkInterfacePrefix,

		"WARDEN_IPTABLES_FILTER_FORWARD_CHAIN=" + config.IPTables.Filter.ForwardChain,
		"WARDEN_IPTABLES_FILTER_DEFAULT_CHAIN=" + config.IPTables.Filter.DefaultChain,
		"WARDEN_IPTABLES_FILTER_INSTANCE_PREFIX=" + config.IPTables.Filter.InstancePrefix,

		"WARDEN_IPTABLES_NAT_PREROUTING_CHAIN=" + config.IPTables.NAT.PreroutingChain,
		"WARDEN_IPTABLES_NAT_POSTROUTING_CHAIN=" + config.IPTables.NAT.PostroutingChain,
		"WARDEN_IPTABLES_NAT_INSTANCE_PREFIX=" + config.IPTables.NAT.InstancePrefix,
	}
}
