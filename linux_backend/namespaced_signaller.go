package linux_backend

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"time"

	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
)

// Kills a process by invoking ./bin/wsh in the given container path using
// a PID read from the given pidFile
type NamespacedSignaller struct {
	Runner        command_runner.CommandRunner
	ContainerPath string
	PidFilePath   string
	Logger        lager.Logger
}

func (n *NamespacedSignaller) Signal(signal os.Signal) error {
	n.Logger.Debug("NamespacedSignaller.Signal-entered", lager.Data{"signal": signal})
	pid, err := pidFromFile(n.PidFilePath)
	if err != nil {
		n.Logger.Error("NamespacedSignaller.Signal-failed-to-read-PID-file", err, lager.Data{"signal": signal})
		return err
	}

	cmd := exec.Command(filepath.Join(n.ContainerPath, "bin/wsh"),
		"--socket", filepath.Join(n.ContainerPath, "run/wshd.sock"),
		"kill", fmt.Sprintf("-%d", signal), fmt.Sprintf("%d", pid))

	n.Logger.Debug("NamespacedSignaller.Signal-about-to-run-kill-command", lager.Data{"signal": signal, "cmd": cmd})
	err = n.Runner.Run(cmd)
	if err != nil {
		n.Logger.Error("NamespacedSignaller.Signal-failed-to-run-kill", err, lager.Data{"signal": signal})
		return err
	}

	n.Logger.Debug("NamespacedSignaller.Signal-ran-kill-successfully", lager.Data{"signal": signal})
	return nil
}

func pidFromFile(pidFilePath string) (int, error) {
	pidFile, err := openPIDFile(pidFilePath)
	if err != nil {
		return 0, err
	}
	defer pidFile.Close()

	fileContent, err := readPIDFile(pidFile)
	if err != nil {
		return 0, err
	}

	var pid int
	_, err = fmt.Sscanf(fileContent, "%d", &pid)
	if err != nil {
		return 0, fmt.Errorf("linux_backend: can't parse PID file content: %v", err)
	}

	return pid, nil
}

func openPIDFile(pidFilePath string) (*os.File, error) {
	var err error

	for i := 0; i < 100; i++ { // 10 seconds
		var pidFile *os.File
		pidFile, err = os.Open(pidFilePath)
		if err == nil {
			return pidFile, nil
		}
		time.Sleep(time.Millisecond * 100)
	}

	return nil, fmt.Errorf("linux_backend: can't open PID file: %s", err)
}

func readPIDFile(pidFile *os.File) (string, error) {
	var bytesReadAmt int

	buffer := make([]byte, 32)
	for i := 0; i < 100; i++ { // retry 10 seconds
		bytesReadAmt, _ = pidFile.Read(buffer)

		if bytesReadAmt == 0 {
			pidFile.Seek(0, 0)
			time.Sleep(time.Millisecond * 100)
			continue
		}
		break
	}

	if bytesReadAmt == 0 {
		return "", errors.New("linux_backend: can't read PID file: is empty or non existent")
	}

	return string(buffer), nil
}
