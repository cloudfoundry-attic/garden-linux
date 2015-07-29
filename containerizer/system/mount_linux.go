package system

import (
	"fmt"
	"os"
	"path/filepath"
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
	Sys              = "sysfs"
)

func (m Mount) Mount() error {
	isDir := true
	sourcePath := m.SourcePath

	if m.SourcePath != "" {
		if info, err := os.Stat(m.SourcePath); err == nil {
			isDir = info.IsDir()
		} else {
			return fmt.Errorf("system: source path stat: %s", err)
		}
	} else {
		sourcePath = string(m.Type)
	}

	rootDir := m.TargetPath
	rootDirPermissions := os.FileMode(0700)

	if !isDir {
		rootDir = filepath.Dir(m.TargetPath)
		rootDirPermissions = os.FileMode(0755)
	}

	if err := os.MkdirAll(rootDir, rootDirPermissions); err != nil {
		return fmt.Errorf("system: create mount point directory %s: %s", rootDir, err)
	}

	if !isDir {
		if _, err := os.OpenFile(m.TargetPath, os.O_CREATE|os.O_RDONLY, 0700); err != nil {
			return fmt.Errorf("system: create mount point file %s: %s", m.TargetPath, err)
		}
	}

	if err := syscall.Mount(sourcePath, m.TargetPath, string(m.Type), uintptr(m.Flags), m.Data); err != nil {
		return fmt.Errorf("system: mount %s on %s: %s", m.Type, m.TargetPath, err)
	}

	return nil
}
