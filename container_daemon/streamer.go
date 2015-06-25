package container_daemon

import (
	"errors"
	"io"
	"os"
	"sync"
	"syscall"

	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_poller/FakePoller.go . Poller
type Poller interface {
	Poll() error
}

type StreamingFile interface {
	io.ReadWriteCloser
	Fd() uintptr
}

type Streamer struct {
	reader StreamingFile
	writer io.Writer

	poller       Poller
	pollChan     chan bool
	stopPollChan chan bool

	stopStreamChan chan bool
	streamMutex    sync.Mutex
	streaming      bool
	bufferSize     int

	logger lager.Logger
}

// Create a streamer which will copy data from the reader to the writer each time the poller returns.
// The streamer will attempt to close both the reader and writer when it stops streaming.
func NewStreamerWithPoller(reader StreamingFile, writer io.Writer, logger lager.Logger, poller Poller) *Streamer {
	return &Streamer{
		reader: reader,
		writer: writer,

		poller:   poller,
		pollChan: make(chan bool),

		stopStreamChan: make(chan bool),
		streaming:      false,
		bufferSize:     64 * 1024,

		logger: logger,
	}
}

// Sets the buffer size for copying.
//
// NOTE: the buffer size must be set (or allowed to default) before starting streaming.
func (s *Streamer) SetBufferSize(bufferSize int) {
	s.streamMutex.Lock()
	defer s.streamMutex.Unlock()
	s.bufferSize = bufferSize
}

// Starts streaming. If already streaming, returns an error.
//
// NOTE: The client must call stop to end streaming.
func (s *Streamer) Start(shouldClose bool) error {
	s.streamMutex.Lock()
	defer s.streamMutex.Unlock()
	if s.streaming {
		return errors.New("container_daemon: streamer already streaming")
	}
	s.streaming = true
	s.stopPollChan = make(chan bool)

	go Poll(s.pollChan, s.stopPollChan, s.poller, s.logger)

	go stream(s.pollChan, s.stopStreamChan, s.reader, s.writer, s.bufferSize, shouldClose, s.logger)

	return nil
}

func Poll(pollChan chan<- bool, stopPollChan <-chan bool, poller Poller, logger lager.Logger) {
	for {
		select {
		case <-stopPollChan:
			close(pollChan)
			return
		default:
		}

		if err := poller.Poll(); err != nil {
			logger.Error("container_deamon streamer poll", err)
			return
		}

		select {
		case pollChan <- true:
		case <-stopPollChan:
			close(pollChan)
			return
		}
	}
}

func stream(pollChan <-chan bool, stopStreamChan chan bool, reader StreamingFile, writer io.Writer,
	bufferSize int, shouldClose bool, logger lager.Logger) {
	var stopping bool
	defer func() {
		if shouldClose {
			reader.Close()
			if closer, ok := writer.(io.Closer); ok {
				closer.Close()
			}
		}

		if !stopping {
			<-stopStreamChan
		}
		stopStreamChan <- true
	}()

	if err := syscall.SetNonblock(int(reader.Fd()), true); err != nil {
		logger.Error("container_deamon streamer set non-blocking", err)
		return
	}

	buffer := make([]byte, bufferSize)
	for !stopping {
		select {
		case <-pollChan:
		case <-stopStreamChan:
			stopping = true
		}

		n, err := reader.Read(buffer)
		if err != nil {
			pErr, ok := err.(*os.PathError)
			if ok && pErr.Err == syscall.EAGAIN && !stopping {
				continue
			}
			logger.Error("container_deamon streamer read", err)
			return
		}

		if _, err := writer.Write(buffer[0:n]); err != nil {
			logger.Error("container_deamon streamer write", err)
			return
		}
	}
}

// Stops streaming after one more pass at copying from the reader to the writer.
// If not streaming, returns an error.
func (s *Streamer) Stop() error {
	s.streamMutex.Lock()
	defer s.streamMutex.Unlock()
	if !s.streaming {
		return errors.New("container_daemon: streamer not streaming")
	}
	s.streaming = false

	close(s.stopPollChan)

	s.stopStreamChan <- true
	<-s.stopStreamChan

	return nil
}
