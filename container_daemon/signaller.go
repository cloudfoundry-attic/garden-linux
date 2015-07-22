package container_daemon

import (
	"fmt"
	"os"
	"syscall"

	"github.com/pivotal-golang/lager"
)

type ProcessSignaller struct {
	Logger lager.Logger
}

func (ps *ProcessSignaller) Signal(pid int, signal syscall.Signal) error {
	logData := lager.Data{"pid": pid, "signal": signal}
	ps.Logger.Debug("ProcessSignaller.Signal-entered", logData)

	process, err := os.FindProcess(pid)

	if err != nil {
		ps.Logger.Debug("ProcessSignaller.Signal-process-not-found", logData)
		return fmt.Errorf("container_daemon: signaller: find process: pid: %d, %s", pid, err)
	} else {
		ps.Logger.Debug("ProcessSignaller.Signal-about-to-signal", logData)
		if err = process.Signal(signal); err != nil {
			ps.Logger.Debug("ProcessSignaller.Signal-failed-to-signal", logData)
			return fmt.Errorf("container_daemon: signaller: signal process: pid: %d, %s", pid, err)
		}

		ps.Logger.Debug("ProcessSignaller.Signal-successfully-signalled", logData)
	}

	return nil
}
