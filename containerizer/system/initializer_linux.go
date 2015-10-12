package system

//go:generate counterfeiter -o fake_step_runner/FakeStepRunner.go . StepRunner
type StepRunner interface {
	Run() error
}

type Initializer struct {
	Steps []StepRunner
}

func (c *Initializer) Init() error {
	for _, step := range c.Steps {
		if err := step.Run(); err != nil {
			return err
		}
	}

	return nil
}
