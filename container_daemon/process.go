package container_daemon

import (
	"fmt"
	"io"

	"github.com/cloudfoundry-incubator/garden"
)

type Process struct {
	pid            int
	exitCodeStream io.Reader
}

//go:generate counterfeiter -o fake_connector/FakeConnector.go . Connector
type Connector interface {
	Connect(msg interface{}) ([]io.ReadWriteCloser, error)
}

func NewProcess(connector Connector, processSpec *garden.ProcessSpec, processIO *garden.ProcessIO) (*Process, error) {
	fds, err := connector.Connect(processSpec)
	if err != nil {
		return nil, fmt.Errorf("container_daemon: connect to socket: %s", err)
	}

	if processIO != nil && processIO.Stdin != nil {
		go io.Copy(fds[0], processIO.Stdin)
	}

	if processIO != nil && processIO.Stdout != nil {
		go io.Copy(processIO.Stdout, fds[1])
	}

	if processIO != nil && processIO.Stderr != nil {
		go io.Copy(processIO.Stderr, fds[2])
	}

	return &Process{}, nil
}

func (p *Process) Pid() int {
	return p.pid
}

func (p *Process) Wait() (int, error) {
	return 0, nil
}
