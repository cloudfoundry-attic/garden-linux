package containerizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	SignalSuccess = iota
	SignalError
)

type Signal struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}

type PipeSynchronizerError struct {
	Message string
}

func (err PipeSynchronizerError) Error() string {
	return err.Message
}

type PipeSynchronizer struct {
	Reader *os.File
	Writer io.Writer
}

func (ps *PipeSynchronizer) Wait(timeout time.Duration) error {
	signalQueue := make(chan Signal)
	errorQueue := make(chan error)

	go func(readerFd uintptr, signalQueue chan Signal, errorQueue chan error) {
		var signal Signal

		file := os.NewFile(readerFd, "/dev/synchronizer-reader")

		decoder := json.NewDecoder(file)
		err := decoder.Decode(&signal)

		if err != nil {
			errorQueue <- err
		} else {
			signalQueue <- signal
		}

	}(ps.Reader.Fd(), signalQueue, errorQueue)

	select {
	case signal := <-signalQueue:
		close(signalQueue)
		close(errorQueue)
		if signal.Type == SignalError {
			return &PipeSynchronizerError{
				Message: signal.Message,
			}
		}
		return nil
	case err := <-errorQueue:
		close(signalQueue)
		close(errorQueue)
		return err
	case <-time.After(timeout):
		return errors.New("synchronizer wait timeout")
	}

	return nil
}

func (ps *PipeSynchronizer) IsSignalError(err error) bool {
	switch err.(type) {
	case *PipeSynchronizerError:
		return true
	default:
		return false
	}
}

func (ps *PipeSynchronizer) SignalSuccess() error {
	signal := Signal{
		Type: SignalSuccess,
	}

	return ps.sendSignal(signal)
}

func (ps *PipeSynchronizer) SignalError(err error) error {
	signal := Signal{
		Type:    SignalError,
		Message: fmt.Sprintf("error: %s", err.Error()),
	}

	return ps.sendSignal(signal)
}

func (ps *PipeSynchronizer) startReader() {
}

func (ps *PipeSynchronizer) sendSignal(signal Signal) error {
	msg, err := json.Marshal(signal)
	if err != nil {
		return err
	}

	_, err = ps.Writer.Write(msg)
	if err != nil {
		return err
	}

	return nil
}
