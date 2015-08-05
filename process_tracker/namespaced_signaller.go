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

	Retry     int
	RetryWait time.Duration
}

func (n *NamespacedSignaller) Signal(request *SignalRequest) error {
	pidfile := path.Join(n.ContainerPath, "processes", fmt.Sprintf("%d.pid", request.Pid))

	n.Logger.Debug("NamespacedSignaller.Signal-entered", lager.Data{"signal": request.Signal})
	pid, err := PidFromFile(pidfile, n.Retry, n.RetryWait)
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

func PidFromFile(pidFilePath string, retries int, retryWait time.Duration) (int, error) {
	pidFile, err := openPIDFile(pidFilePath, retries, retryWait)
	if err != nil {
		return 0, err
	}
	defer pidFile.Close()

	fileContent, err := readPIDFile(pidFile, retries, retryWait)
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

func openPIDFile(pidFilePath string, retries int, retryWait time.Duration) (*os.File, error) {
	var err error

	for i := 0; i < retries; i++ {
		var pidFile *os.File
		pidFile, err = os.Open(pidFilePath)
		if err == nil {
			return pidFile, nil
		}

		time.Sleep(retryWait)
	}

	return nil, fmt.Errorf("linux_backend: can't open PID file: %s", err)
}

func readPIDFile(pidFile *os.File, retries int, retryWait time.Duration) (string, error) {
	var bytesReadAmt int

	buffer := make([]byte, 32)
	for i := 0; i < retries; i++ {
		bytesReadAmt, _ = pidFile.Read(buffer)

		if bytesReadAmt == 0 {
			pidFile.Seek(0, 0)
			time.Sleep(retryWait)

			continue
		}
		break
	}

	if bytesReadAmt == 0 {
		return "", errors.New("linux_backend: can't read PID file: is empty or non existent")
	}

	return string(buffer), nil
}
