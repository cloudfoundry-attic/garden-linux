package process_tracker

import (
	"fmt"
	"syscall"

	"github.com/pivotal-golang/lager"
)

type ExtraFdSignaller struct {
	Logger lager.Logger
}

func (e *ExtraFdSignaller) Signal(signal *SignalRequest) error {
	message := `{"Signal":"%s"}`
	switch signal.Signal {
	case syscall.SIGKILL:
		message = fmt.Sprintf(message, "KILL")
	case syscall.SIGTERM:
		message = fmt.Sprintf(message, "TERM")
	default:
		return fmt.Errorf("process_tracker: ExtraFdSignaller failed to send signal: unknown signal: %d", signal)
	}
	return signal.Link.SendExtraFdMsg([]byte(message))
}
