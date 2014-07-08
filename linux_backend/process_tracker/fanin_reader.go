package process_tracker

import (
	"errors"
	"io"
	"sync"
)

type faninReader struct {
	io.Reader

	w      io.WriteCloser
	closed bool
	writeL sync.Mutex
}

func (r *faninReader) Write(data []byte) (int, error) {
	r.writeL.Lock()

	if r.closed {
		return 0, errors.New("write after close")
	}

	defer r.writeL.Unlock()

	return r.w.Write(data)
}

func (r *faninReader) Close() error {
	r.writeL.Lock()

	if r.closed {
		return errors.New("closed twice")
	}

	r.closed = true

	defer r.writeL.Unlock()

	return r.w.Close()
}

func (r *faninReader) AddSource(sink io.Reader) {
	go func() {
		io.Copy(r, sink)
		r.Close()
	}()
}
