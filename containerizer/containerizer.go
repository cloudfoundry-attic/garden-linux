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
		return fmt.Errorf("containerizer: Failed to run `parent-before-clone`: %s", err)
	}

	// TODO: Set hard rlimits

	pid, err := c.Execer.Exec(c.InitBinPath, c.InitArgs...)
	if err != nil {
		return fmt.Errorf("containerizer: Failed to create container: %s", err)
	}

	// Temporary until we merge the hook scripts functionality in Golang
	os.Setenv("PID", strconv.Itoa(pid))
	cmd = exec.Command(path.Join(c.LibPath, "hook"), "parent-after-clone")
	if err := c.CommandRunner.Run(cmd); err != nil {
		return fmt.Errorf("containerizer: Failed to run `parent-after-clone`: %s", err)
	}

	if err := c.Signaller.SignalSuccess(); err != nil {
		return fmt.Errorf("containerizer: Failed to send success singnal to the container: %s", err)
	}

	if err := c.Waiter.Wait(timeout); err != nil {
		return fmt.Errorf("containerizer: Failed to wait for container: %s", err)
	}

	return nil
}

func (c *Containerizer) Run() error {
	if err := c.Waiter.Wait(timeout); err != nil {
		err = fmt.Errorf("containerizer: Failed to wait for host: %s", err)
		c.Signaller.SignalError(err)
		return err
	}

	if err := c.Daemon.Init(); err != nil {
		err = fmt.Errorf("containerizer: Failed to initialize daemon: %s", err)
		c.Signaller.SignalError(err)
		return err
	}

	if err := c.RootFS.Enter(); err != nil {
		err = fmt.Errorf("containerizer: Failed to enter root fs: %s", err)
		c.Signaller.SignalError(err)
		return err
	}

	// TODO: TTY stuff (ptmx)

	if err := c.Initializer.Init(); err != nil {
		err = fmt.Errorf("containerizer: initializing the container: %s", err)
		c.Signaller.SignalError(err)
		return err
	}

	if err := c.Signaller.SignalSuccess(); err != nil {
		err = fmt.Errorf("containerizer: Failed to signal host: %s", err)
		c.Signaller.SignalError(err)
		return err
	}

	if err := c.Daemon.Run(); err != nil {
		err = fmt.Errorf("containerizer: Failed to run daemon: %s", err)
		c.Signaller.SignalError(err)
		return err
	}

	return nil
}
