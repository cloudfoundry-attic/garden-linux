package iptables

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"

	"github.com/cloudfoundry-incubator/garden/api"
	"github.com/cloudfoundry/gunk/command_runner"
)

type Chain interface {
	AppendRule(source string, destination string, jump Action) error
	DeleteRule(source string, destination string, jump Action) error

	AppendNatRule(source string, destination string, jump Action, to net.IP) error
	DeleteNatRule(source string, destination string, jump Action, to net.IP) error

	PrependFilterRule(protocol api.Protocol, dest string, destPort uint32) error
}

func NewChain(name string, runner command_runner.CommandRunner) Chain {
	return &chain{Name: name, Runner: runner}
}

type chain struct {
	Name   string
	Runner command_runner.CommandRunner
}

func (ch *chain) AppendRule(source string, destination string, jump Action) error {
	return ch.Create(&rule{
		Source:      source,
		Destination: destination,
		Jump:        jump,
	})
}

func (ch *chain) DeleteRule(source string, destination string, jump Action) error {
	return ch.Destroy(&rule{
		Source:      source,
		Destination: destination,
		Jump:        jump,
	})
}

func (ch *chain) AppendNatRule(source string, destination string, jump Action, to net.IP) error {
	return ch.Create(&rule{
		Type:        Nat,
		Source:      source,
		Destination: destination,
		Jump:        jump,
		To:          to,
	})
}

func (ch *chain) DeleteNatRule(source string, destination string, jump Action, to net.IP) error {
	return ch.Destroy(&rule{
		Type:        Nat,
		Source:      source,
		Destination: destination,
		Jump:        jump,
		To:          to,
	})
}

func (ch *chain) PrependFilterRule(protocol api.Protocol, dest string, destPort uint32) error {
	parms := []string{"-w", "-I", ch.Name, "1"}

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

	return ch.Runner.Run(exec.Command("/sbin/iptables", parms...))
}

type rule struct {
	Type        Type
	Source      string
	Destination string
	To          net.IP
	Jump        Action
}

func (n *rule) create(chain string, runner command_runner.CommandRunner) error {
	return runner.Run(exec.Command("/sbin/iptables", flags("-A", chain, n)...))
}

func (n *rule) destroy(chain string, runner command_runner.CommandRunner) error {
	return runner.Run(exec.Command("/sbin/iptables", flags("-D", chain, n)...))
}

func flags(action, chain string, n *rule) []string {
	rule := []string{"-w"}

	if n.Type != "" {
		rule = append(rule, "-t", string(n.Type))
	}

	rule = append(rule, action, chain)

	if n.Source != "" {
		rule = append(rule, "--source", n.Source)
	}

	if n.Destination != "" {
		rule = append(rule, "--destination", n.Destination)
	}

	rule = append(rule, "--jump", string(n.Jump))

	if n.To != nil {
		rule = append(rule, "--to", string(n.To.String()))
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
	return rule.create(c.Name, c.Runner)
}

func (c *chain) Destroy(rule destroyer) error {
	return rule.destroy(c.Name, c.Runner)
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
