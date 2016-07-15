package container_daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"code.cloudfoundry.org/garden"
)

const DefaultRootPATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
const DefaultUserPath = "/usr/local/bin:/usr/bin:/bin"

const (
	ProcessRequest = iota
	ProcessResponse
	SignalRequest
	SignalResponse
)

type RequestMessage struct {
	Type int
	Data json.RawMessage
}

type ResponseMessage struct {
	Type       int
	Files      []StreamingFile `json:"-"`
	Pid        int
	ErrMessage string
}

type StreamingFile interface {
	io.ReadWriteCloser
	Fd() uintptr
}

//go:generate counterfeiter -o fake_connection_handler/FakeConnectionHandler.go . ConnectionHandler
type ConnectionHandler interface {
	Handle(decoder *json.Decoder) (*ResponseMessage, error)
}

//go:generate counterfeiter -o fake_listener/FakeListener.go . Listener
type Listener interface {
	Listen(ch ConnectionHandler) error
	Close() error
}

//go:generate counterfeiter -o fake_cmdpreparer/fake_cmdpreparer.go . CmdPreparer
type CmdPreparer interface {
	PrepareCmd(garden.ProcessSpec) (*exec.Cmd, error)
}

//go:generate counterfeiter -o fake_spawner/FakeSpawner.go . Spawner
type Spawner interface {
	Spawn(cmd *exec.Cmd, withTty bool) ([]*os.File, error)
}

//go:generate counterfeiter -o fake_signaller/FakeSignaller.go . Signaller
type Signaller interface {
	Signal(pid int, signal syscall.Signal) error
}

type ContainerDaemon struct {
	CmdPreparer CmdPreparer
	Spawner     Spawner
	Signaller   Signaller
}

type SignalSpec struct {
	Pid    int
	Signal syscall.Signal
}

func (cd *ContainerDaemon) Run(listener Listener) error {
	if err := listener.Listen(cd); err != nil {
		return fmt.Errorf("container_daemon: listening for connections: %s", err)
	}

	return nil
}

func (cd *ContainerDaemon) Handle(decoder *json.Decoder) (response *ResponseMessage, err error) {
	defer func() {
		if recoveredErr := recover(); recoveredErr != nil {
			err = fmt.Errorf("container_daemon: recovered panic: %s", recoveredErr)
		}
	}()

	var cmd *exec.Cmd
	var request RequestMessage
	err = decoder.Decode(&request)
	if err != nil {
		return nil, fmt.Errorf("container_daemon: decode message spec: %s", err)
	}

	response = &ResponseMessage{}

	switch request.Type {
	case ProcessRequest:
		var spec garden.ProcessSpec

		if err := json.Unmarshal(request.Data, &spec); err != nil {
			return nil, fmt.Errorf("container_daemon: json unmarshal process spec: %s", err)
		}

		if cmd, err = cd.CmdPreparer.PrepareCmd(spec); err != nil {
			return nil, err
		}

		var files []*os.File
		if files, err = cd.Spawner.Spawn(cmd, spec.TTY != nil); err != nil {
			return nil, err
		}
		if len(files) > 0 {
			response.Files = make([]StreamingFile, len(files))
			for i, f := range files {
				response.Files[i] = f
			}
		}

		response.Type = ProcessResponse
		response.Pid = cmd.Process.Pid

	case SignalRequest:
		var spec SignalSpec

		if err := json.Unmarshal(request.Data, &spec); err != nil {
			return nil, fmt.Errorf("container_daemon: json unmarshal signal spec: %s", err)
		}

		if err := cd.Signaller.Signal(spec.Pid, spec.Signal); err != nil {
			return nil, err
		}

		response.Type = SignalResponse

	default:
		return nil, fmt.Errorf("container_daemon: unknown message: %s", request)
	}

	return
}
