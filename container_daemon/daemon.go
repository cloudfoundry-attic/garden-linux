package container_daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden"
)

const DefaultRootPATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
const DefaultUserPath = "/usr/local/bin:/usr/bin:/bin"

//go:generate counterfeiter -o fake_connection_handler/FakeConnectionHandler.go . ConnectionHandler
type ConnectionHandler interface {
	Handle(decoder *json.Decoder) ([]*os.File, int, error)
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

type ContainerDaemon struct {
	CmdPreparer CmdPreparer

	Spawner Spawner
}

func (cd *ContainerDaemon) Run(listener Listener) error {
	if err := listener.Listen(cd); err != nil {
		return fmt.Errorf("container_daemon: listening for connections: %s", err)
	}

	return nil
}

func (cd *ContainerDaemon) Handle(decoder *json.Decoder) (fds []*os.File, pid int, err error) {
	defer func() {
		if recoveredErr := recover(); recoveredErr != nil {
			err = fmt.Errorf("container_daemon: recovered panic: %s", recoveredErr)
		}
	}()

	var spec garden.ProcessSpec
	err = decoder.Decode(&spec)
	if err != nil {
		return nil, 0, fmt.Errorf("container_daemon: decode process spec: %s", err)
	}

	var cmd *exec.Cmd
	cmd, err = cd.CmdPreparer.PrepareCmd(spec)
	if err != nil {
		return nil, 0, err
	}

	fds, err = cd.Spawner.Spawn(cmd, spec.TTY != nil)
	if err != nil {
		return nil, 0, err
	}

	return fds, cmd.Process.Pid, err
}
