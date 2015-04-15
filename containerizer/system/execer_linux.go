package system

import (
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry/gunk/command_runner"
)

type Execer struct {
	CommandRunner command_runner.CommandRunner
	Stdout        io.Writer
	Stderr        io.Writer
	ExtraFiles    []*os.File
}

func (e *Execer) Exec(binPath string, args ...string) (int, error) {
	cmd := exec.Command(binPath, args...)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: uintptr(syscall.CLONE_NEWUTS | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID),
	}

	cmd.Stdout = e.Stdout
	cmd.Stderr = e.Stderr
	cmd.ExtraFiles = e.ExtraFiles

	e.CommandRunner.Start(cmd)

	return cmd.Process.Pid, nil
}
