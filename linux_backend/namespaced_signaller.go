package linux_backend

import (
	"errors"
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

	fileContent, err := readPIDFile(pidFile)

	if err != nil {
		return err
	}

	var pid int
	_, err = fmt.Sscanf(fileContent, "%d", &pid)
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

func readPIDFile(pidFile *os.File) (string, error) {
	var err error
	var bytesReadAmt int

	buffer := make([]byte, 32)
	for i := 0; i < 100; i++ { // retry 10 seconds
		bytesReadAmt, err = pidFile.Read(buffer)
		if err != nil {
			return "", fmt.Errorf("namespaced-signaller: can't read pidfile: %v", err)
		}
		if bytesReadAmt == 0 {
			pidFile.Seek(0, 0)
			time.Sleep(time.Millisecond * 100)
			continue
		}
		break
	}

	if bytesReadAmt == 0 {
		return "", errors.New("namespaced-signaller: can't read pidfile: is empty")
	}

	return string(buffer), nil
}
