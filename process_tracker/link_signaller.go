package process_tracker

import (
	"syscall"

	"github.com/pivotal-golang/lager"
)

type LinkSignaller struct {
	Logger lager.Logger
}

func (ls *LinkSignaller) Signal(request *SignalRequest) error {
	data := lager.Data{"pid": request.Pid, "signal": request.Signal}

	signal := request.Signal
	if signal == syscall.SIGKILL {
		signal = syscall.SIGUSR1
	}

	ls.Logger.Debug("LinkSignaller.Signal-about-to-signal", data)
	if err := request.Link.SendSignal(signal); err != nil {
		ls.Logger.Error("LinkSignaller.Signal-failed-to-signal", err, data)
		return err
	}

	ls.Logger.Debug("LinkSignaller.Signal-signal-succeeded", data)
	return nil
}
