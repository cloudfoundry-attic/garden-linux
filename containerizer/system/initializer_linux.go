package system

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

//go:generate counterfeiter -o fake_configurer/FakeConfigurer.go . Configurer
type Configurer interface {
	Configure() error
}

type Initializer struct {
	Root              string
	NetworkConfigurer Configurer
}

func (i *Initializer) Init() error {
	// syscall.Setuid(0)
	// syscall.Setgid(0)

	i.NetworkConfigurer.Configure()
	if err := i.mountTmp(); err != nil {
		panic(err)
	}

	return nil
}

// Pre-condition: /proc must exist.
func (*Initializer) mountProc() error {
	if err := syscall.Mount("proc", filepath.Join(i.Root, "proc"), "proc", uintptr(0), ""); err != nil {
		return fmt.Errorf("linux_backend: MountProc: %s", err)
	}
	return nil
}

func (i *Initializer) mountTmp() error {
	shmdir := filepath.join(i.root, "dev/shm")
	if err := os.MkdirAll(shmdir, 0700); err != nil {
		return err
	}
	if err := syscall.mount("tmpfs", shmdir, "tmpfs", uintptr(syscall.ms_nodev), ""); err != nil {
		return fmt.errorf("linux_backend: mounttmp: %s", err)
	}

	return nil
}
