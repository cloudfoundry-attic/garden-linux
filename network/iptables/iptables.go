package iptables

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
)

var protocols = map[garden.Protocol]string{
	garden.ProtocolAll:  "all",
	garden.ProtocolTCP:  "tcp",
	garden.ProtocolICMP: "icmp",
	garden.ProtocolUDP:  "udp",
}

// NewGlobalChain creates a chain without an associated log chain.
// The chain is not created by this package (currently it is created in net.sh).
// It is an error to attempt to call Setup on this chain.
func NewGlobalChain(name string, runner command_runner.CommandRunner, log lager.Logger) Chain {
	return &chain{name: name, logChainName: "", runner: runner, logger: log}
}

// NewLoggingChain creates a chain with an associated log chain.
// This allows NetOut calls with the 'log' parameter to succesfully log.
func NewLoggingChain(name string, useKernelLogging bool, runner command_runner.CommandRunner, logger lager.Logger) Chain {
	return &chain{name: name, logChainName: name + "-log", useKernelLogging: useKernelLogging, runner: runner, logger: logger}
}

//go:generate counterfeiter . Chain
type Chain interface {
	// Create the actual iptable chains in the underlying system.
	// logPrefix defines the log prefix used for logging this chain.
	Setup(logPrefix string) error

	// Destroy the actual iptable chains in the underlying system
	TearDown() error

	AppendRule(source string, destination string, jump Action) error
	DeleteRule(source string, destination string, jump Action) error

	AppendNatRule(source string, destination string, jump Action, to net.IP) error
	DeleteNatRule(source string, destination string, jump Action, to net.IP) error

	PrependFilterRule(rule garden.NetOutRule) error
}

type chain struct {
	name             string
	logChainName     string
	useKernelLogging bool
	runner           command_runner.CommandRunner
	logger           lager.Logger
}

func (ch *chain) Setup(logPrefix string) error {
	if ch.logChainName == "" {
		// we still use net.sh to set up global non-logging chains
		panic("cannot set up chains without associated log chains")
	}

	ch.TearDown()

	if err := ch.runner.Run(exec.Command("/sbin/iptables", "-w", "-N", ch.logChainName)); err != nil {
		return fmt.Errorf("iptables: log chain setup: %v", err)
	}

	logParams := ch.buildLogParams(logPrefix)
	appendFlags := []string{"-w", "-A", ch.logChainName, "-m", "conntrack", "--ctstate", "NEW,UNTRACKED,INVALID", "--protocol", "tcp"}
	if err := ch.runner.Run(exec.Command("/sbin/iptables", append(appendFlags, logParams...)...)); err != nil {
		return fmt.Errorf("iptables: log chain setup: %v", err)
	}

	if err := ch.runner.Run(exec.Command("/sbin/iptables", "-w", "-A", ch.logChainName, "--jump", "RETURN")); err != nil {
		return fmt.Errorf("iptables: log chain setup: %v", err)
	}

	return nil
}

func (ch *chain) buildLogParams(logPrefix string) []string {
	if ch.useKernelLogging {
		return []string{"--jump", "LOG", "--log-prefix", fmt.Sprintf("%s ", logPrefix)}
	} else {
		return []string{"--jump", "NFLOG", "--nflog-prefix", fmt.Sprintf("%s ", logPrefix), "--nflog-group", "1"}
	}
}

func (ch *chain) TearDown() error {
	if ch.logChainName == "" {
		// we still use net.sh to tear down global non-logging chains
		panic("cannot tear down chains without associated log chains")
	}

	ch.runner.Run(exec.Command("/sbin/iptables", "-w", "-F", ch.logChainName))
	ch.runner.Run(exec.Command("/sbin/iptables", "-w", "-X", ch.logChainName))
	return nil
}

func (ch *chain) AppendRule(source string, destination string, jump Action) error {
	return ch.Create(&rule{
		source:      source,
		destination: destination,
		jump:        jump,
	})
}

func (ch *chain) DeleteRule(source string, destination string, jump Action) error {
	return ch.Destroy(&rule{
		source:      source,
		destination: destination,
		jump:        jump,
	})
}

func (ch *chain) AppendNatRule(source string, destination string, jump Action, to net.IP) error {
	return ch.Create(&rule{
		typ:         Nat,
		source:      source,
		destination: destination,
		jump:        jump,
		to:          to,
	})
}

func (ch *chain) DeleteNatRule(source string, destination string, jump Action, to net.IP) error {
	return ch.Destroy(&rule{
		typ:         Nat,
		source:      source,
		destination: destination,
		jump:        jump,
		to:          to,
	})
}

type singleRule struct {
	Protocol garden.Protocol
	Networks *garden.IPRange
	Ports    *garden.PortRange
	ICMPs    *garden.ICMPControl
	Log      bool
}

func (ch *chain) PrependFilterRule(r garden.NetOutRule) error {

	if len(r.Ports) > 0 && !allowsPort(r.Protocol) {
		return fmt.Errorf("Ports cannot be specified for Protocol %s", strings.ToUpper(protocols[r.Protocol]))
	}

	single := singleRule{
		Protocol: r.Protocol,
		ICMPs:    r.ICMPs,
		Log:      r.Log,
	}

	// It should still loop once even if there are no networks or ports.
	for j := 0; j < len(r.Networks) || j == 0; j++ {
		for i := 0; i < len(r.Ports) || i == 0; i++ {

			// Preserve nils unless there are ports specified
			if len(r.Ports) > 0 {
				single.Ports = &r.Ports[i]
			}

			// Preserve nils unless there are networks specified
			if len(r.Networks) > 0 {
				single.Networks = &r.Networks[j]
			}

			if err := ch.prependSingleRule(single); err != nil {
				return err
			}
		}
	}

	return nil
}

func allowsPort(p garden.Protocol) bool {
	return p == garden.ProtocolTCP || p == garden.ProtocolUDP
}

func (ch *chain) prependSingleRule(r singleRule) error {
	params := []string{"-w", "-I", ch.name, "1"}

	protocolString, ok := protocols[r.Protocol]

	if !ok {
		return fmt.Errorf("invalid protocol: %d", r.Protocol)
	}

	params = append(params, "--protocol", protocolString)

	network := r.Networks
	if network != nil {
		if network.Start != nil && network.End != nil {
			params = append(params, "-m", "iprange", "--dst-range", network.Start.String()+"-"+network.End.String())
		} else if network.Start != nil {
			params = append(params, "--destination", network.Start.String())
		} else if network.End != nil {
			params = append(params, "--destination", network.End.String())
		}
	}

	ports := r.Ports
	if ports != nil {
		if ports.End != ports.Start {
			params = append(params, "--destination-port", fmt.Sprintf("%d:%d", ports.Start, ports.End))
		} else {
			params = append(params, "--destination-port", fmt.Sprintf("%d", ports.Start))
		}
	}

	if r.ICMPs != nil {
		icmpType := fmt.Sprintf("%d", r.ICMPs.Type)
		if r.ICMPs.Code != nil {
			icmpType = fmt.Sprintf("%d/%d", r.ICMPs.Type, *r.ICMPs.Code)
		}

		params = append(params, "--icmp-type", icmpType)
	}

	if r.Log {
		params = append(params, "--goto", ch.logChainName)
	} else {
		params = append(params, "--jump", "RETURN")
	}

	ch.logger.Debug("prepend-filter-rule", lager.Data{"parms": params})

	var stderr bytes.Buffer
	cmd := exec.Command("/sbin/iptables", params...)
	cmd.Stderr = &stderr
	if err := ch.runner.Run(cmd); err != nil {
		return fmt.Errorf("iptables: %v, %v", err, stderr.String())
	}

	return nil
}

type rule struct {
	typ         Type
	source      string
	destination string
	to          net.IP
	jump        Action
}

func (n *rule) create(chain string, runner command_runner.CommandRunner) error {
	return runner.Run(exec.Command("/sbin/iptables", flags("-A", chain, n)...))
}

func (n *rule) destroy(chain string, runner command_runner.CommandRunner) error {
	return runner.Run(exec.Command("/sbin/iptables", flags("-D", chain, n)...))
}

func flags(action, chain string, n *rule) []string {
	rule := []string{"-w"}

	if n.typ != "" {
		rule = append(rule, "-t", string(n.typ))
	}

	rule = append(rule, action, chain)

	if n.source != "" {
		rule = append(rule, "--source", n.source)
	}

	if n.destination != "" {
		rule = append(rule, "--destination", n.destination)
	}

	rule = append(rule, "--jump", string(n.jump))

	if n.to != nil {
		rule = append(rule, "--to", string(n.to.String()))
	}

	return rule
}

type Destroyable interface {
	Destroy() error
}

type creater interface {
	create(chain string, runner command_runner.CommandRunner) error
}

type destroyer interface {
	destroy(chain string, runner command_runner.CommandRunner) error
}

func (c *chain) Create(rule creater) error {
	return rule.create(c.name, c.runner)
}

func (c *chain) Destroy(rule destroyer) error {
	return rule.destroy(c.name, c.runner)
}

type Action string

const (
	Return    Action = "RETURN"
	SourceNAT        = "SNAT"
	Reject           = "REJECT"
	Drop             = "DROP"
)

type Type string

const (
	Nat Type = "nat"
)
