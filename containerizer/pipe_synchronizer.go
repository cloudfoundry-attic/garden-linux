package containerizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
	"io"
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
	Reader io.Reader
	Writer io.Writer
}

func (ps *PipeSynchronizer) Wait(timeout time.Duration) error {
	signalQueue := make(chan Signal)
	errorQueue := make(chan error)

	go func(reader io.Reader, signalQueue chan Signal, errorQueue chan error) {
		var signal Signal

		decoder := json.NewDecoder(reader)
		err := decoder.Decode(&signal)

		if err != nil {
			errorQueue <- err
		} else {
			signalQueue <- signal
		}
	}(ps.Reader, signalQueue, errorQueue)

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
