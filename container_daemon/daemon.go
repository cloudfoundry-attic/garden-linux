package container_daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry/gunk/command_runner"
)

//go:generate counterfeiter -o fake_listener/FakeListener.go . Listener
type Listener interface {
	Init() error
	Listen(ch unix_socket.ConnectionHandler) error
	Stop() error
}

type ContainerDaemon struct {
	Listener Listener
	Runner   command_runner.CommandRunner
}

// This method should be called from the host namespace, to open the socket file in the right file system.
func (cd *ContainerDaemon) Init() error {
	if err := cd.Listener.Init(); err != nil {
		return fmt.Errorf("container_daemon: initializing the listener: %s", err)
	}

	return nil
}

func (cd *ContainerDaemon) Run() error {
	if err := cd.Listener.Listen(cd); err != nil {
		return fmt.Errorf("container_daemon: listening for connections: %s", err)
	}

	return nil
}

func (cd *ContainerDaemon) Handle(decoder *json.Decoder) ([]*os.File, error) {
	var spec garden.ProcessSpec

	decoder.Decode(&spec)

	var pipes [3]struct {
		r *os.File
		w *os.File
	}
	for i := 0; i < 3; i++ {
		pipes[i].r, pipes[i].w, _ = os.Pipe()
	}

	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Stdin = pipes[0].r
	cmd.Stdout = pipes[1].w
	defer pipes[1].w.Close()
	cmd.Stderr = pipes[2].w
	defer pipes[2].w.Close()

	if err := cd.Runner.Start(cmd); err != nil {
		return nil, fmt.Errorf("running command: %s", err)
	}

	// Hint: use goroutine to wait for process, write error code to extra fd and clean up the pipes

	return []*os.File{pipes[0].w, pipes[1].r, pipes[2].r}, nil
}

func (cd *ContainerDaemon) Stop() error {
	if err := cd.Listener.Stop(); err != nil {
		return fmt.Errorf("container_daemon: stoping the listener: %s", err)
	}

	return nil
}
