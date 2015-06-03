package system_test

import (
	"io"
	"os/exec"
	"syscall"

	"fmt"
)

func runInContainer(stdout, stderr io.Writer, privileged bool, programName string, args ...string) error {
	var container string

	// Locate appropriate binary.
	// Note: gexec.Build must be run in the suite rather than in the test to avoid intermittent failures
	// due to racing builds.
	switch programName {
	case "fake_mounter":
		container = fakeMounterBin
	case "fake_container":
		container = fakeContainerBin
	default:
		return fmt.Errorf("Unexpected programName %q", programName)
	}

	flags := syscall.CLONE_NEWNS
	flags = flags | syscall.CLONE_NEWUTS
	if !privileged {
		flags = flags | syscall.CLONE_NEWUSER
	}

	cmd := exec.Command(container, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: uintptr(flags),
	}

	if !privileged {
		cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      0,
				Size:        1,
			},
		}
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      0,
				Size:        1,
			},
		}
	}

	return cmd.Run()
}
