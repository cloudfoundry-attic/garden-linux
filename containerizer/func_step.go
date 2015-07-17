package containerizer

import "errors"

type FuncStep struct {
	Func func() error
}

func (s *FuncStep) Run() error {
	if s.Func == nil {
		return errors.New("containerizer: callback function is not defined")
	}
	return s.Func()
}
