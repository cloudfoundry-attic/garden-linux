package system_test

import (
	"io"
	"os/exec"
	"syscall"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func runInContainer(stdout, stderr io.Writer, privileged bool, programName string, args ...string) error {
	container, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/containerizer/system/" + programName)
	Expect(err).ToNot(HaveOccurred())

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
