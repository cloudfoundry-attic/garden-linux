package process_tracker

import (
	"encoding/json"
	"fmt"

	"github.com/cloudfoundry-incubator/garden-linux/iodaemon/link"
	"github.com/pivotal-golang/lager"
)

type LinkSignaller struct {
	Logger lager.Logger
}

func (e *LinkSignaller) Signal(signal *SignalRequest) error {
	data, err := json.Marshal(&link.SignalMsg{Signal: signal.Signal})
	if err != nil {
		return fmt.Errorf("process_tracker: %s", data)
	}
	return signal.Link.SendMsg(data)
}
