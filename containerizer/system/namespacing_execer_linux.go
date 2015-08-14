package system

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	coresys "github.com/cloudfoundry-incubator/garden-linux/system"
	"github.com/cloudfoundry/gunk/command_runner"
)

const UIDMappingRange = 65536

type NamespacingExecer struct {
	CommandRunner command_runner.CommandRunner
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

		mapping, err := makeSysProcIDMap()
		if err != nil {
			return 0, err
		}
		cmd.SysProcAttr.UidMappings = mapping
		cmd.SysProcAttr.GidMappings = mapping

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

func makeSysProcIDMap() ([]syscall.SysProcIDMap, error) {
	mappingList, err := coresys.NewMappingList()
	if err != nil {
		return nil, err
	}

	mapping := []syscall.SysProcIDMap{}
	for _, entry := range mappingList {
		mapping = append(mapping, syscall.SysProcIDMap{
			ContainerID: entry.FromID,
			HostID:      entry.ToID,
			Size:        entry.Size,
		})
	}

	return mapping, nil
}
