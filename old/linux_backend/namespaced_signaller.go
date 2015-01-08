package linux_backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudfoundry/gunk/command_runner"
)

// Kills a process by invoking ./bin/wsh in the given container path using
// a PID read from the given pidFile
type NamespacedSignaller struct {
	Runner        command_runner.CommandRunner
	ContainerPath string
	PidFilePath   string
}

func (n *NamespacedSignaller) Signal(signal os.Signal) error {
	pidfile, err := os.Open(n.PidFilePath)
	if err != nil {
		return fmt.Errorf("namespaced-signaller: can't read pidfile: %v", err)
	}

	defer pidfile.Close()

	var pid int
	_, err = fmt.Fscanf(pidfile, "%d", &pid)
	if err != nil {
		return fmt.Errorf("namespaced-signaller: can't read pidfile: %v", err)
	}

	return n.Runner.Run(exec.Command(filepath.Join(n.ContainerPath, "bin/wsh"),
		"--socket", filepath.Join(n.ContainerPath, "run/wshd.sock"),
		"kill", fmt.Sprintf("-%d", signal), fmt.Sprintf("%d", pid)))
}
