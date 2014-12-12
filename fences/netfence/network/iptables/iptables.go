package iptables

import (
	"net"
	"os/exec"

	"github.com/cloudfoundry/gunk/command_runner"
)

type Chain struct {
	Name   string
	Runner command_runner.CommandRunner
}

type Rule struct {
	Type        Type
	Source      string
	Destination string
	To          net.IP
	Jump        Action
}

func (n *Rule) create(chain string, runner command_runner.CommandRunner) error {
	return runner.Run(exec.Command("/sbin/iptables", flags("-A", chain, n)...))
}

func (n *Rule) destroy(chain string, runner command_runner.CommandRunner) error {
	return runner.Run(exec.Command("/sbin/iptables", flags("-D", chain, n)...))
}

func flags(action, chain string, n *Rule) []string {
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

func (c *Chain) Create(rule creater) error {
	return rule.create(c.Name, c.Runner)
}

func (c *Chain) Destroy(rule destroyer) error {
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
