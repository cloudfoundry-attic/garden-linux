package system

import (
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry/gunk/command_runner"
)

type NamespacingExecer struct {
	CommandRunner command_runner.CommandRunner
	Stdout        io.Writer
	Stderr        io.Writer
	ExtraFiles    []*os.File
	Privileged    bool
}

func (e *NamespacingExecer) Exec(binPath string, args ...string) (int, error) {
	cmd := exec.Command(binPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	flags := syscall.CLONE_NEWIPC
	flags = flags | syscall.CLONE_NEWNET
	flags = flags | syscall.CLONE_NEWNS
	flags = flags | syscall.CLONE_NEWUTS
	flags = flags | syscall.CLONE_NEWPID

	if !e.Privileged {
		flags = flags | syscall.CLONE_NEWUSER

		cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      0,
				Size:        100000,
			},
		}
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      0,
				Size:        100000,
			},
		}
	}

	cmd.SysProcAttr.Cloneflags = uintptr(flags)
	cmd.ExtraFiles = e.ExtraFiles

	e.CommandRunner.Start(cmd)

	return cmd.Process.Pid, nil
}
