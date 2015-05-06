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

	oldroot, err := os.Open("/")
	if err != nil {
		return fmt.Errorf("system: failed to open old root filesystem: %s", err)
	}
	defer oldroot.Close() // Ignore error

	// Hack: PivotRoot requires r.Root to be a file system, so bind mount r.Root
	// to itself so r.Root appears to be a file system.
	if err := syscall.Mount(r.Root, r.Root, "", uintptr(syscall.MS_BIND|syscall.MS_REC), ""); err != nil {
		return fmt.Errorf("system: failed to bind mount the root filesystem onto itself: %s", err)
	}

	rootfs, err := os.Open(r.Root)
	if err != nil {
		return fmt.Errorf("system: failed to open root filesystem: %s", err)
	}
	defer rootfs.Close() // Ignore error

	if err := rootfs.Chdir(); err != nil {
		return fmt.Errorf("system: failed to change directory into the bind mounted rootfs dir: %s", err)
	}

	if err := syscall.PivotRoot(".", "."); err != nil {
		return fmt.Errorf("system: failed to pivot root: %s", err)
	}

	if err := oldroot.Chdir(); err != nil {
		return fmt.Errorf("system: failed to change directory into the old root filesystem: %s", err)
	}

	if err := syscall.Unmount(".", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("system: failed to unmount the old root filesystem: %s", err)
	}

	if err := rootfs.Chdir(); err != nil {
		return fmt.Errorf("system: failed to change directory into the root filesystem: %s", err)
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
