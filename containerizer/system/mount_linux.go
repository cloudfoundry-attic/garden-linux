package system

import (
	"fmt"
	"os"
	"syscall"
)

type Mount struct {
	Type  MountType
	Path  string
	Flags int
}

type MountType string

const (
	Tmpfs MountType = "tmpfs"
	Proc            = "proc"
)

func (m Mount) Mount() error {
	if err := syscall.Setuid(0); err != nil {
		return fmt.Errorf("system: failed to setuid: %s", err)
	}

	if err := syscall.Setgid(0); err != nil {
		return fmt.Errorf("system: failed to setgid: %s", err)
	}

	if err := os.MkdirAll(m.Path, 0700); err != nil {
		return fmt.Errorf("system: create mount point directory %s: %s", m.Path, err)
	}

	if err := syscall.Mount(string(m.Type), m.Path, string(m.Type), uintptr(m.Flags), ""); err != nil {
		return fmt.Errorf("system: mount %s on %s: %s", m.Type, m.Path, err)
	}

	return nil
}
