package containerizer

import "fmt"

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
	Start() error
}

type Containerizer struct {
	InitBinPath string
	Execer      ContainerExecer
	RootFS      RootFSEnterer
	SetUider    SetUider
}

func (c *Containerizer) Create() error {
	_, err := c.Execer.Exec(c.InitBinPath)
	if err != nil {
		return fmt.Errorf("containerizer: Failed to create container: %s", err)
	}

	return nil
}

func (c *Containerizer) Child() error {
	if err := c.RootFS.Enter(); err != nil {
		return fmt.Errorf("containerizer: Failed to enter root fs: %s", err)
	}

	// TODO: TTY stuff (ptmx)

	if err := c.SetUider.SetUid(); err != nil {
		return fmt.Errorf("containerizer: Failed to set uid: %s", err)
	}

	// TODO: Call child-after-pivot hook scripts

	// TODO: Unmount old root

	// TODO: Barrier(s) for synchronization with tha parent

	// TODO: Run the daemon

	return nil
}
