package containerizer

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"time"

	"github.com/cloudfoundry/gunk/command_runner"
)

var timeout = time.Second * 3

//go:generate counterfeiter -o fake_container_execer/FakeContainerExecer.go . ContainerExecer
type ContainerExecer interface {
	Exec(binPath string, args ...string) (int, error)
}

//go:generate counterfeiter -o fake_rootfs_enterer/FakeRootFSEnterer.go . RootFSEnterer
type RootFSEnterer interface {
	Enter() error
}

//go:generate counterfeiter -o fake_container_initializer/FakeContainerInitializer.go . ContainerInitializer
type ContainerInitializer interface {
	Init() error
}

//go:generate counterfeiter -o fake_container_daemon/FakeContainerDaemon.go . ContainerDaemon
type ContainerDaemon interface {
	Init() error
	Run() error
}

//go:generate counterfeiter -o fake_signaller/FakeSignaller.go . Signaller
type Signaller interface {
	SignalError(err error) error
	SignalSuccess() error
}

//go:generate counterfeiter -o fake_waiter/FakeWaiter.go . Waiter
type Waiter interface {
	Wait(timeout time.Duration) error
	IsSignalError(err error) bool
}

type Containerizer struct {
	InitBinPath string
	InitArgs    []string
	Execer      ContainerExecer
	RootFS      RootFSEnterer
	Initializer ContainerInitializer
	Daemon      ContainerDaemon
	Signaller   Signaller
	Waiter      Waiter
	// Temporary until we merge the hook scripts functionality in Golang
	CommandRunner command_runner.CommandRunner
	LibPath       string
}

func (c *Containerizer) Create() error {
	// Temporary until we merge the hook scripts functionality in Golang
	cmd := exec.Command(path.Join(c.LibPath, "hook"), "parent-before-clone")
	if err := c.CommandRunner.Run(cmd); err != nil {
		return fmt.Errorf("containerizer: run `parent-before-clone`: %s", err)
	}

	// TODO: Set hard rlimits

	pid, err := c.Execer.Exec(c.InitBinPath, c.InitArgs...)
	if err != nil {
		return fmt.Errorf("containerizer: create container: %s", err)
	}

	// Temporary until we merge the hook scripts functionality in Golang
	err = os.Setenv("PID", strconv.Itoa(pid))
	if err != nil {
		return fmt.Errorf("containerizer: failed to set PID env var: %s", err)
	}

	cmd = exec.Command(path.Join(c.LibPath, "hook"), "parent-after-clone")
	if err := c.CommandRunner.Run(cmd); err != nil {
		return fmt.Errorf("containerizer: run `parent-after-clone`: %s", err)
	}

	if err := c.Signaller.SignalSuccess(); err != nil {
		return fmt.Errorf("containerizer: send success singnal to the container: %s", err)
	}

	if err := c.Waiter.Wait(timeout); err != nil {
		return fmt.Errorf("containerizer: wait for container: %s", err)
	}

	return nil
}

func (c *Containerizer) Run() error {
	if err := c.Waiter.Wait(timeout); err != nil {
		return c.signalErrorf("containerizer: wait for host: %s", err)
	}

	if err := c.Daemon.Init(); err != nil {
		return c.signalErrorf("containerizer: initialize daemon: %s", err)
	}

	if err := c.RootFS.Enter(); err != nil {
		return c.signalErrorf("containerizer: enter root fs: %s", err)
	}

	// TODO: TTY stuff (ptmx)

	if err := c.Initializer.Init(); err != nil {
		return c.signalErrorf("containerizer: initializing the container: %s", err)
	}

	if err := c.Signaller.SignalSuccess(); err != nil {
		return c.signalErrorf("containerizer: signal host: %s", err)
	}

	if err := c.Daemon.Run(); err != nil {
		return c.signalErrorf("containerizer: run daemon: %s", err)
	}

	return nil
}

func (c *Containerizer) signalErrorf(format string, err error) error {
	err = fmt.Errorf(format, err)

	if signalErr := c.Signaller.SignalError(err); signalErr != nil {
		err = fmt.Errorf("containerizer: signal error: %s (while signalling %s)", signalErr, err)
	}
	return err
}
