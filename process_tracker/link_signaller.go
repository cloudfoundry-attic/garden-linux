package process_tracker

import "github.com/pivotal-golang/lager"

type LinkSignaller struct {
	Logger lager.Logger
}

func (ls *LinkSignaller) Signal(request *SignalRequest) error {
	data := lager.Data{"pid": request.Pid, "signal": request.Signal}

	ls.Logger.Debug("ProcessSignaller.Signal-about-to-signal", data)
	if err := request.Link.SendSignal(request.Signal); err != nil {
		ls.Logger.Error("ProcessSignaller.Signal-failed-to-signal", err, data)
		return err
	}
	ls.Logger.Debug("ProcessSignaller.Signal-signal-succeeded", data)
	return nil
}
