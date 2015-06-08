package system

import (
	"fmt"
	"os"
	"syscall"
)

type RootFS struct {
	Root string
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func (r *RootFS) Enter() error {
	if err := checkDirectory(r.Root); err != nil {
		return err
	}

	if err := syscall.Mount(r.Root, r.Root, "", uintptr(syscall.MS_BIND|syscall.MS_REC), ""); err != nil {
		return fmt.Errorf("system: failed to bind mount the root filesystem onto itself: %s", err)
	}

	if err := os.Chdir(r.Root); err != nil {
		return fmt.Errorf("system: failed to change directory into the bind mounted rootfs dir: %s", err)
	}

	if err := os.MkdirAll("tmp/garden-host", 0700); err != nil {
		return fmt.Errorf("system: mkdir: %s", err)
	}

	if err := syscall.PivotRoot(".", "tmp/garden-host"); err != nil {
		return fmt.Errorf("system: failed to pivot root: %s", err)
	}

	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("system: failed to chdir to new root: %s", err)
	}

	return nil
}

func checkDirectory(dir string) error {
	if fi, err := os.Stat(dir); err != nil {
		return fmt.Errorf("system: validate root file system: %v", err)
	} else if !fi.IsDir() {
		return fmt.Errorf("system: validate root file system: %s is not a directory", dir)
	}

	return nil
}
