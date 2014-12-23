package iptables

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"

	"github.com/cloudfoundry-incubator/garden/api"
	"github.com/cloudfoundry/gunk/command_runner"
)

type ChainFactory interface {
	CreateChain(name string) Chain
}

func NewChainFactory(runner command_runner.CommandRunner) ChainFactory {
	return &chainFactory{runner}
}

type chainFactory struct {
	runner command_runner.CommandRunner
}

func (cf *chainFactory) CreateChain(name string) Chain {
	return &chain{name: name, runner: cf.runner}
}

type Chain interface {
	AppendRule(source string, destination string, jump Action) error
	DeleteRule(source string, destination string, jump Action) error

	AppendNatRule(source string, destination string, jump Action, to net.IP) error
	DeleteNatRule(source string, destination string, jump Action, to net.IP) error

	PrependFilterRule(protocol api.Protocol, dest string, destPort uint32) error
}

type chain struct {
	name   string
	runner command_runner.CommandRunner
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

func (ch *chain) PrependFilterRule(protocol api.Protocol, dest string, destPort uint32) error {
	parms := []string{"-w", "-I", ch.name, "1"}

	protocols := map[api.Protocol]string{
		api.ProtocolAll: "all",
		api.ProtocolTCP: "tcp",
	}
	protocolString, ok := protocols[protocol]

	if !ok {
		return fmt.Errorf("invalid protocol: %d", protocol)
	}

	parms = append(parms, "--protocol", protocolString)

	if dest != "" {
		parms = append(parms, "--destination", dest)
	}

	if destPort != 0 {
		parms = append(parms, "--destination-port", strconv.Itoa(int(destPort)))
	}

	parms = append(parms, "--jump", "RETURN")

	return ch.runner.Run(exec.Command("/sbin/iptables", parms...))
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
