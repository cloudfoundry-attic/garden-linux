package container_daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/cloudfoundry/gunk/command_runner"
)

//go:generate counterfeiter -o fake_listener/FakeListener.go . Listener
type Listener interface {
	Init() error
	Listen(ch unix_socket.ConnectionHandler) error
	Stop() error
}

//go:generate counterfeiter -o fake_exit_checker/FakeExitChecker.go . ExitChecker
type Waiter interface {
	Wait(cmd *exec.Cmd) (byte, error)
}

type ContainerDaemon struct {
	Listener Listener
	Users    system.User
	Runner   command_runner.CommandRunner
	Waiter   Waiter
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

	var pipes [4]struct {
		r *os.File
		w *os.File
	}

	for i := 0; i < 4; i++ {
		pipes[i].r, pipes[i].w, _ = os.Pipe()
	}

	// defer pipes[1].w.Close()
	// defer pipes[2].w.Close()

	var uid, gid uint32
	if user, err := cd.Users.Lookup(spec.User); err == nil && user != nil {
		fmt.Sscanf(user.Uid, "%d", &uid) // todo(jz): handle errors
		fmt.Sscanf(user.Gid, "%d", &gid)
	} else if err == nil {
		return nil, fmt.Errorf("container_daemon: failed to lookup user %s", spec.User)
	} else {
		return nil, fmt.Errorf("container_daemon: lookup user %s: %s", spec.User, err)
	}

	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uid,
			Gid: gid,
		},
	}

	cmd.Stdin = pipes[0].r
	cmd.Stdout = pipes[1].w
	cmd.Stderr = pipes[2].w

	if err := cd.Runner.Start(cmd); err != nil {
		return nil, fmt.Errorf("container_daemon: running command: %s", err)
	}

	go func(runner command_runner.CommandRunner, exit Waiter, cmd *exec.Cmd) {
		e, _ := exit.Wait(cmd)
		defer pipes[3].w.Close()

		pipes[3].w.Write([]byte{e})
	}(cd.Runner, cd.Waiter, cmd)

	// Hint: use goroutine to wait for process, write error code to extra fd and clean up the pipes

	return []*os.File{pipes[0].w, pipes[1].r, pipes[2].r, pipes[3].r}, nil
}

func (cd *ContainerDaemon) Stop() error {
	if err := cd.Listener.Stop(); err != nil {
		return fmt.Errorf("container_daemon: stopping the listener: %s", err)
	}

	return nil
}
