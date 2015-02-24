package process_tracker

import (
	"errors"
	"io"
	"sync"
)

type faninWriter struct {
	w      io.WriteCloser
	closed bool
	writeL sync.Mutex

	hasSink chan struct{}
}

func (fw *faninWriter) Write(data []byte) (int, error) {
	<-fw.hasSink

	fw.writeL.Lock()
	defer fw.writeL.Unlock()

	if fw.closed {
		return 0, errors.New("write after close")
	}

	return fw.w.Write(data)
}

func (fw *faninWriter) Close() error {
	<-fw.hasSink

	fw.writeL.Lock()
	defer fw.writeL.Unlock()

	if fw.closed {
		return errors.New("closed twice")
	}

	fw.closed = true

	return fw.w.Close()
}

//AddSink can only be called once
func (fw *faninWriter) AddSink(sink io.WriteCloser) {
	fw.w = sink
	close(fw.hasSink)
}

func (fw *faninWriter) AddSource(source io.Reader) {
	go func() {
		_, err := io.Copy(fw, source)
		if err == nil {
			fw.Close()
		}
	}()
}
