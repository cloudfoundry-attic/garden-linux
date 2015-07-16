package system

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry/gunk/command_runner"
)

const UIDMappingRange = 65536

type NamespacingExecer struct {
	CommandRunner    command_runner.CommandRunner
	ExtraFiles       []*os.File
	Privileged       bool
	UidMappingOffset int
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
				HostID:      e.UidMappingOffset,
				Size:        UIDMappingRange,
			},
		}
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      e.UidMappingOffset,
				Size:        UIDMappingRange,
			},
		}

		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: 0,
			Gid: 0,
		}
	}

	cmd.SysProcAttr.Cloneflags = uintptr(flags)
	cmd.ExtraFiles = e.ExtraFiles

	if err := e.CommandRunner.Start(cmd); err != nil {
		return 0, fmt.Errorf("system: failed to start the supplied command: %s", err)
	}

	return cmd.Process.Pid, nil
}
