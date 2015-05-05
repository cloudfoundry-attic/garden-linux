package container_daemon

import (
	"fmt"
	"io"

	"github.com/cloudfoundry-incubator/garden"
)

const UnknownExitStatus = 255

type Process struct {
	pid int

	exitCode <-chan int
}

//go:generate counterfeiter -o fake_connector/FakeConnector.go . Connector
type Connector interface {
	Connect(msg interface{}) ([]io.ReadWriteCloser, int, error)
}

func NewProcess(connector Connector, processSpec *garden.ProcessSpec, processIO *garden.ProcessIO) (*Process, error) {
	fds, pid, err := connector.Connect(processSpec)
	if err != nil {
		return nil, fmt.Errorf("container_daemon: connect to socket: %s", err)
	}

	if processIO != nil && processIO.Stdin != nil {
		go io.Copy(fds[0], processIO.Stdin) // Ignore error
	}

	if processIO != nil && processIO.Stdout != nil {
		go io.Copy(processIO.Stdout, fds[1]) // Ignore error
	}

	if processIO != nil && processIO.Stderr != nil {
		go io.Copy(processIO.Stderr, fds[2]) // Ignore error
	}

	exitChan := make(chan int)
	go func(exitFd io.Reader, exitChan chan<- int, processIO *garden.ProcessIO) {
		b := make([]byte, 1)
		_, err := exitFd.Read(b)
		if err != nil {
			b[0] = UnknownExitStatus

			if processIO != nil && processIO.Stderr != nil { // This will only be false in tests
				fmt.Fprintf(processIO.Stderr, "container_daemon: failed to read exit status: %s", err) // Ignore error
			}
		}
		exitChan <- int(b[0])
	}(fds[3], exitChan, processIO)

	return &Process{pid: pid, exitCode: exitChan}, nil
}

func (p *Process) Pid() int {
	return p.pid
}

func (p *Process) Wait() (int, error) {
	return <-p.exitCode, nil
}
