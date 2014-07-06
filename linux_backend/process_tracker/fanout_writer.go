package process_tracker

import (
	"errors"
	"io"
	"sync"
)

type fanoutWriter struct {
	sinks  []io.WriteCloser
	closed bool
	sinksL sync.Mutex
}

func (w *fanoutWriter) Write(data []byte) (int, error) {
	w.sinksL.Lock()

	if w.closed {
		return 0, errors.New("write after close")
	}

	// the sinks should be nonblocking and never actually error;
	// we can assume lossiness here, and do this all within the lock
	for _, s := range w.sinks {
		s.Write(data)
	}

	w.sinksL.Unlock()

	return len(data), nil
}

func (w *fanoutWriter) Close() error {
	w.sinksL.Lock()

	if w.closed {
		return errors.New("closed twice")
	}

	for _, s := range w.sinks {
		s.Close()
	}

	w.closed = true
	w.sinks = nil

	w.sinksL.Unlock()

	return nil
}

func (w *fanoutWriter) AddSink(sink io.WriteCloser) {
	w.sinksL.Lock()

	if w.closed {
		sink.Close()
	} else {
		w.sinks = append(w.sinks, sink)
	}

	w.sinksL.Unlock()
}
