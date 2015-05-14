package container_daemon

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
)

const DefaultRootPATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
const DefaultUserPath = "/usr/local/bin:/usr/bin:/bin"

//go:generate counterfeiter -o fake_listener/FakeListener.go . Listener
type Listener interface {
	Init() error
	Listen(ch unix_socket.ConnectionHandler) error
	Stop() error
}

//go:generate counterfeiter -o fake_spawner/FakeSpawner.go . Spawner
type Spawner interface {
	Spawn(spec garden.ProcessSpec) ([]*os.File, int, error)
}

type ContainerDaemon struct {
	Listener Listener
	Spawner  Spawner
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

	return cd.Spawner.Spawn(spec)
}

func (cd *ContainerDaemon) Stop() error {
	if err := cd.Listener.Stop(); err != nil {
		return fmt.Errorf("container_daemon: stopping the listener: %s", err)
	}

	return nil
}
