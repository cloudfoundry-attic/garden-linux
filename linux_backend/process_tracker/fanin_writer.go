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
}

func (w *faninWriter) Write(data []byte) (int, error) {
	w.writeL.Lock()

	if w.closed {
		return 0, errors.New("write after close")
	}

	defer w.writeL.Unlock()

	return w.w.Write(data)
}

func (w *faninWriter) Close() error {
	w.writeL.Lock()

	if w.closed {
		return errors.New("closed twice")
	}

	w.closed = true

	defer w.writeL.Unlock()

	return w.w.Close()
}

func (w *faninWriter) AddSource(sink io.Reader) {
	go func() {
		io.Copy(w, sink)
		w.Close()
	}()
}
