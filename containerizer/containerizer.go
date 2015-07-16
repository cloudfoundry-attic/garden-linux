package containerizer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cloudfoundry/gunk/command_runner"
)

var timeout = time.Second * 15

//go:generate counterfeiter -o fake_rlimits_initializer/FakeRlimitsInitializer.go . RlimitsInitializer
type RlimitsInitializer interface {
	Init() error
}

//go:generate counterfeiter -o fake_container_execer/FakeContainerExecer.go . ContainerExecer
type ContainerExecer interface {
	Exec(binPath string, args ...string) (int, error)
}

//go:generate counterfeiter -o fake_rootfs_enterer/FakeRootFSEnterer.go . RootFSEnterer
type RootFSEnterer interface {
	Enter() error
}

//go:generate counterfeiter -o fake_initializer/FakeInitializer.go . Initializer
type Initializer interface {
	Init() error
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
	Rlimits              RlimitsInitializer
	InitBinPath          string
	InitArgs             []string
	Execer               ContainerExecer
	RootfsPath           string
	ContainerInitializer Initializer
	Signaller            Signaller
	Waiter               Waiter
	// Temporary until we merge the hook scripts functionality in Golang
	CommandRunner command_runner.CommandRunner
	LibPath       string
}

func (c *Containerizer) Create() error {
	if err := c.Rlimits.Init(); err != nil {
		return fmt.Errorf("containerizer: initializing resource limits: %s", err)
	}

	// Temporary until we merge the hook scripts functionality in Golang
	cmd := exec.Command(path.Join(c.LibPath, "hook"), "parent-before-clone")
	if err := c.CommandRunner.Run(cmd); err != nil {
		return fmt.Errorf("containerizer: run `parent-before-clone`: %s", err)
	}

	pid, err := c.Execer.Exec(c.InitBinPath, c.InitArgs...)
	if err != nil {
		return fmt.Errorf("containerizer: create container: %s", err)
	}

	// Temporary until we merge the hook scripts functionality in Golang
	err = os.Setenv("PID", strconv.Itoa(pid))
	if err != nil {
		return fmt.Errorf("containerizer: failed to set PID env var: %s", err)
	}

	var stderr bytes.Buffer
	cmd = exec.Command(path.Join(c.LibPath, "hook"), "parent-after-clone")
	cmd.Stderr = &stderr
	if err := c.CommandRunner.Run(cmd); err != nil {
		return fmt.Errorf("containerizer: run `parent-after-clone`: %s. stderr: %s", err, stderr.String())
	}

	pivotter := exec.Command(filepath.Join(c.LibPath, "pivotter"), "-rootfs", c.RootfsPath)
	pivotter.Env = append(pivotter.Env, fmt.Sprintf("TARGET_NS_PID=%d", pid))
	if err := c.CommandRunner.Run(pivotter); err != nil {
		return fmt.Errorf("containerizer: run pivotter: %s", err)
	}

	if err := c.Signaller.SignalSuccess(); err != nil {
		return fmt.Errorf("containerizer: send success singnal to the container: %s", err)
	}

	if err := c.Waiter.Wait(timeout); err != nil {
		return fmt.Errorf("containerizer: wait for container: %s", err)
	}

	return nil
}

func (c *Containerizer) Init() error {
	if err := c.ContainerInitializer.Init(); err != nil {
		return c.signalErrorf("containerizer: initializing the container: %s", err)
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
