package process_tracker

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
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
	Logger        lager.Logger
	Timeout       time.Duration
}

func (n *NamespacedSignaller) Signal(request *SignalRequest) error {
	pidfile := path.Join(n.ContainerPath, "processes", fmt.Sprintf("%d.pid", request.Pid))

	n.Logger.Debug("NamespacedSignaller.Signal-entered", lager.Data{"signal": request.Signal})
	pid, err := PidFromFile(pidfile, n.Timeout)
	if err != nil {
		n.Logger.Error("NamespacedSignaller.Signal-failed-to-read-PID-file", err, lager.Data{"signal": request.Signal})
		return err
	}

	cmd := exec.Command(filepath.Join(n.ContainerPath, "bin/wsh"),
		"--socket", filepath.Join(n.ContainerPath, "run/wshd.sock"),
		"--user", "root",
		"kill", fmt.Sprintf("-%d", request.Signal), fmt.Sprintf("%d", pid))

	n.Logger.Debug("NamespacedSignaller.Signal-about-to-run-kill-command", lager.Data{"signal": request.Signal, "cmd": cmd})
	err = n.Runner.Run(cmd)
	if err != nil {
		n.Logger.Error("NamespacedSignaller.Signal-failed-to-run-kill", err, lager.Data{"signal": request.Signal})
		return err
	}

	n.Logger.Debug("NamespacedSignaller.Signal-ran-kill-successfully", lager.Data{"signal": request.Signal})
	return nil
}

func PidFromFile(pidFilePath string, timeout time.Duration) (int, error) {
	pidFile, err := openPIDFile(pidFilePath, timeout)
	if err != nil {
		return 0, err
	}
	defer pidFile.Close()

	fileContent, err := readPIDFile(pidFile, timeout)
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

func openPIDFile(pidFilePath string, timeout time.Duration) (*os.File, error) {
	var err error

	iterationCount := timeout / (time.Millisecond * 100)
	for i := 0; i < int(iterationCount); i++ {
		var pidFile *os.File
		pidFile, err = os.Open(pidFilePath)
		if err == nil {
			return pidFile, nil
		}
		time.Sleep(time.Millisecond * 100)
	}

	return nil, fmt.Errorf("linux_backend: can't open PID file: %s", err)
}

func readPIDFile(pidFile *os.File, timeout time.Duration) (string, error) {
	var bytesReadAmt int

	buffer := make([]byte, 32)
	iterationCount := timeout / (time.Millisecond * 100)
	for i := 0; i < int(iterationCount); i++ {
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
