package containerizer

import "fmt"

//go:generate counterfeiter -o fake_capabilities/FakeCapabilities.go . Capabilities
type Capabilities interface {
	Limit(bool) error
}

type CapabilitiesStep struct {
	Drop         bool
	Capabilities Capabilities
}

func (step *CapabilitiesStep) Run() error {
	if !step.Drop {
		return nil
	}

	if err := step.Capabilities.Limit(false); err != nil {
		return fmt.Errorf("containerizer: limitting capabilities: %s\n", err)
	}

	return nil
}
