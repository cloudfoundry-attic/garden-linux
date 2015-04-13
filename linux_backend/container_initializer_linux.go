package linux_backend

import (
	"fmt"
	"syscall"
)

type containerInitializer struct{}

func NewContainerInitializer() ContainerInitializer {
	return &containerInitializer{}
}

func (*containerInitializer) SetHostname(hostname string) error {
	return syscall.Sethostname([]byte(hostname))
}

// Pre-condition: /proc must exist.
func (*containerInitializer) MountProc() error {
	if err := syscall.Mount("proc", "/proc", "proc", uintptr(0), ""); err != nil {
		return fmt.Errorf("linux_backend: MountProc: %s", err)
	}
	return nil
}

func (*containerInitializer) MountTmp() error {
	if err := syscall.Mount("tmpfs", "/dev/shm", "tmpfs", uintptr(syscall.MS_NODEV), ""); err != nil {
		return fmt.Errorf("linux_backend: MountTmp: %s", err)
	}
	return nil
}
