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
	Privileged    bool
}

func (e *Execer) Exec(binPath string, args ...string) (int, error) {
	cmd := exec.Command(binPath, args...)

	flags := syscall.CLONE_NEWIPC
	flags = flags | syscall.CLONE_NEWNET
	flags = flags | syscall.CLONE_NEWNS
	flags = flags | syscall.CLONE_NEWUTS
	flags = flags | syscall.CLONE_NEWPID

	if !e.Privileged {
		flags = flags | syscall.CLONE_NEWUSER
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: uintptr(flags),
	}

	cmd.Stdout = e.Stdout
	cmd.Stderr = e.Stderr
	cmd.ExtraFiles = e.ExtraFiles

	e.CommandRunner.Start(cmd)

	return cmd.Process.Pid, nil
}
