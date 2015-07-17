package system

import (
	"fmt"
	"os"
	"syscall"
)

type Mount struct {
	Type       MountType
	SourcePath string
	TargetPath string
	Flags      int
	Data       string
}

type MountType string

const (
	Tmpfs  MountType = "tmpfs"
	Proc             = "proc"
	Devpts           = "devpts"
	Bind             = "bind"
)

func (m Mount) Mount() error {
	if err := os.MkdirAll(m.TargetPath, 0700); err != nil {
		return fmt.Errorf("system: create mount point directory %s: %s", m.TargetPath, err)
	}

	if err := syscall.Mount(string(m.Type), m.TargetPath, string(m.Type), uintptr(m.Flags), m.Data); err != nil {
		return fmt.Errorf("system: mount %s on %s: %s", m.Type, m.TargetPath, err)
	}

	return nil
}
