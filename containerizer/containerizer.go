package containerizer

import (
	"fmt"
	"time"
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

//go:generate counterfeiter -o fake_set_uider/SetUider.go . SetUider
type SetUider interface {
	SetUid() error
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
	SetUider    SetUider
	Daemon      ContainerDaemon
	Signaller   Signaller
	Waiter      Waiter
}

func (c *Containerizer) Create() error {
	// TODO: Call parent-before-clone

	// TODO: Set hard rlimits

	_, err := c.Execer.Exec(c.InitBinPath, c.InitArgs...)
	if err != nil {
		return fmt.Errorf("containerizer: Failed to create container: %s", err)
	}

	// TODO: Export PID environment variable

	// TODO: Call parent-after-clone

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

	// if err := c.SetUider.SetUid(); err != nil {
	// 	containerSide.Write([]byte(fmt.Sprintf("containerizer: Failed to set uid: %s", err)))
	// 	return fmt.Errorf("containerizer: Failed to set uid: %s", err)
	// }

	// TODO: Call child-after-pivot hook scripts

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
