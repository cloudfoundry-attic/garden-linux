package process_tracker

import (
	"github.com/cloudfoundry-incubator/garden/warden"
)

type namedStream struct {
	process *Process
	source  warden.ProcessStreamSource
}

func newNamedStream(process *Process, source warden.ProcessStreamSource) *namedStream {
	return &namedStream{
		process: process,
		source:  source,
	}
}

func (s *namedStream) Write(data []byte) (int, error) {
	myBytes := make([]byte, len(data))
	copy(myBytes, data)
	s.process.sendToStreams(warden.ProcessStream{
		Source: s.source,
		Data:   myBytes,
	})

	return len(data), nil
}
