package container_daemon

import (
	"fmt"
	"os"
	"os/exec"
)

type ShellRunnerStep struct {
	Runner Runner
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

	if status := step.Runner.Wait(command); status != 0 {
		return fmt.Errorf("expected command %s to exit zero, it exited %d", step.Path, status)
	}

	return nil
}
