package system

import (
	"os/exec"
	"syscall"

	"github.com/cloudfoundry/gunk/command_runner"
)

type Execer struct {
	CommandRunner command_runner.CommandRunner
}

func (e Execer) Exec(binPath string, args ...string) (int, error) {
	cmd := exec.Command(binPath, args...)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: uintptr(syscall.CLONE_NEWUTS | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID),
	}

	e.CommandRunner.Run(cmd)

	return cmd.Process.Pid, nil
}
