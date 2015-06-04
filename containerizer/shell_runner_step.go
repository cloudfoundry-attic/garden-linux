package containerizer

import (
	"fmt"
	"os"
	"os/exec"
	"github.com/cloudfoundry/gunk/command_runner"
)

type ShellRunnerStep struct {
	Runner command_runner.CommandRunner
	Path   string
}

func (step *ShellRunnerStep) Init() error {
	if _, err := os.Stat(step.Path); os.IsNotExist(err) {
		return nil
	}

	command := exec.Command("sh", step.Path)
	if err := step.Runner.Start(command); err != nil {
		return fmt.Errorf("starting command %s: %s", step.Path, err)
	}

	if err := step.Runner.Wait(command); err != nil {
		return fmt.Errorf("runnng command %s: %s", step.Path, err)
	}

	return nil
}
