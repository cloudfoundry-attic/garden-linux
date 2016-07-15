package sysconfig

import (
	"os"
	"os/exec"

	"code.cloudfoundry.org/garden-linux/process"
	"github.com/cloudfoundry/gunk/command_runner"
)

type runner struct {
	command_runner.CommandRunner

	env process.Env
}

func NewRunner(config Config, commandRunner command_runner.CommandRunner) command_runner.CommandRunner {
	return &runner{
		CommandRunner: commandRunner,

		env: config.Environ(),
	}
}

func (runner *runner) Run(cmd *exec.Cmd) error {
	return runner.CommandRunner.Run(runner.withEnv(cmd))
}

func (runner *runner) Start(cmd *exec.Cmd) error {
	return runner.CommandRunner.Start(runner.withEnv(cmd))
}

func (runner *runner) Background(cmd *exec.Cmd) error {
	return runner.CommandRunner.Background(runner.withEnv(cmd))
}

func (runner *runner) withEnv(cmd *exec.Cmd) *exec.Cmd {
	if len(cmd.Env) == 0 {
		cmd.Env = append(os.Environ(), runner.env.Array()...)
	} else {
		cmd.Env = append(cmd.Env, runner.env.Array()...)
	}

	return cmd
}
