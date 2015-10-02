package iptables_manager

import (
	"net"

	"os/exec"

	"fmt"

	"bytes"
	"io/ioutil"

	"github.com/cloudfoundry-incubator/garden-linux/sysconfig"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
)

type filterChain struct {
	cfg    *sysconfig.IPTablesFilterConfig
	runner command_runner.CommandRunner
	logger lager.Logger
}

func NewFilterChain(cfg *sysconfig.IPTablesFilterConfig, runner command_runner.CommandRunner, logger lager.Logger) *filterChain {
	return &filterChain{
		cfg:    cfg,
		runner: runner,
		logger: logger,
	}
}

func (mgr *filterChain) Setup(containerID, bridgeName string, ip net.IP, network *net.IPNet) error {
	instanceChain := mgr.cfg.InstancePrefix + containerID

	commands := []*exec.Cmd{
		// Create filter instance chain
		exec.Command("iptables", "--wait", "-N", instanceChain),
		// Allow intra-subnet traffic (Linux ethernet bridging goes through ip stack)
		exec.Command("iptables", "--wait", "-A", instanceChain, "-s", network.String(), "-d", network.String(), "-j", "ACCEPT"),
		// Otherwise, use the default filter chain
		exec.Command("iptables", "--wait", "-A", instanceChain, "-goto", mgr.cfg.DefaultChain),
		// Bind filter instance chain to filter forward chain
		exec.Command("iptables", "--wait", "-I", mgr.cfg.ForwardChain, "2", "--in-interface", bridgeName, "--source", ip.String(), "--goto", instanceChain),
	}

	for _, cmd := range commands {
		buffer := &bytes.Buffer{}
		cmd.Stderr = buffer
		if err := mgr.runner.Run(cmd); err != nil {
			stderr, _ := ioutil.ReadAll(buffer)
			mgr.logger.Error("setup", err, lager.Data{"cmd": cmd, "stderr": string(stderr)})
			return fmt.Errorf("iptables_manager: filter: %s", err)
		}
	}

	return nil
}

func (mgr *filterChain) Teardown(containerID string) error {
	instanceChain := mgr.cfg.InstancePrefix + containerID

	commands := []*exec.Cmd{
		// Prune forward chain
		exec.Command("sh", "-c", fmt.Sprintf(
			`iptables --wait -S %s 2> /dev/null |
 grep "\-g %s \b" | sed -e "s/-A/-D/" | xargs --no-run-if-empty --max-lines=1 iptables --wait`,
			mgr.cfg.ForwardChain, instanceChain,
		)),
		// Flush instance chain
		exec.Command("sh", "-c", fmt.Sprintf("iptables --wait -F %s 2> /dev/null || true", instanceChain)),
		// Delete instance chain
		exec.Command("sh", "-c", fmt.Sprintf("iptables --wait -X %s 2> /dev/null || true", instanceChain)),
	}

	for _, cmd := range commands {
		if err := mgr.runner.Run(cmd); err != nil {
			return fmt.Errorf("iptables_manager: %s", err)
		}
	}

	return nil
}
