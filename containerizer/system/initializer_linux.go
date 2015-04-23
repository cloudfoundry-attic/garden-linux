package system

import (
	"fmt"
	"syscall"
)

type Configurer interface {
	Configure() error
}

type Initializer struct {
	NetworkConfigurer Configurer
}

func (i *Initializer) Init() error {
	// syscall.Setuid(0)
	// syscall.Setgid(0)

	i.NetworkConfigurer.Configure()

	return nil
}

// Pre-condition: /proc must exist.
func (*Initializer) mountProc() error {
	if err := syscall.Mount("proc", "/proc", "proc", uintptr(0), ""); err != nil {
		return fmt.Errorf("linux_backend: MountProc: %s", err)
	}
	return nil
}

func (*Initializer) mountTmp() error {
	if err := syscall.Mount("tmpfs", "/dev/shm", "tmpfs", uintptr(syscall.MS_NODEV), ""); err != nil {
		return fmt.Errorf("linux_backend: MountTmp: %s", err)
	}
	return nil
}
