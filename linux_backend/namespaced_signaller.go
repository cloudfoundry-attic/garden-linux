package linux_backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"time"

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
	pidFile, err := openPIDFile(n.PidFilePath)
	if err != nil {
		return err
	}
	defer pidFile.Close()

	var pid int
	_, err = fmt.Fscanf(pidFile, "%d", &pid)
	if err != nil {
		return fmt.Errorf("namespaced-signaller: can't read pidfile: %v", err)
	}

	return n.Runner.Run(exec.Command(filepath.Join(n.ContainerPath, "bin/wsh"),
		"--socket", filepath.Join(n.ContainerPath, "run/wshd.sock"),
		"kill", fmt.Sprintf("-%d", signal), fmt.Sprintf("%d", pid)))
}

func openPIDFile(pidFileName string) (*os.File, error) {
	var err error

	for i := 0; i < 100; i++ { // 10 seconds
		var pidFile *os.File
		pidFile, err = os.Open(pidFileName)
		if err == nil {
			return pidFile, nil
		}
		time.Sleep(time.Millisecond * 100)
	}

	return nil, fmt.Errorf("linux_backend: namespaced-signaller can't open PID file: %s", err)
}
