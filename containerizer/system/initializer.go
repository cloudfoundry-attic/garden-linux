package system

//go:generate counterfeiter -o fake_initializer/FakeInitializer.go . Initializer
type Initializer interface {
	Init() error
}

type ContainerInitializer struct {
	Steps []Initializer
}

func (c *ContainerInitializer) Init() error {
	for _, step := range c.Steps {
		if err := step.Init(); err != nil {
			return err
		}
	}

	return nil
}
