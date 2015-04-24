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

	oldroot, _ := os.Open("/")
	defer oldroot.Close()

	rootfs, _ := os.Open(r.Root)
	defer rootfs.Close()

	syscall.Mount(r.Root, r.Root, "", uintptr(syscall.MS_BIND|syscall.MS_REC), "")
	rootfs.Chdir()
	syscall.PivotRoot(".", ".")

	oldroot.Chdir()
	syscall.Unmount(".", syscall.MNT_DETACH)

	rootfs.Chdir()

	return nil
}

func checkDirectory(dir string) error {
	if fi, err := os.Stat(dir); err != nil {
		return fmt.Errorf("containerizer: validate root file system: %v", err)
	} else if !fi.IsDir() {
		return fmt.Errorf("containerizer: validate root file system: %s is not a directory", dir)
	}

	return nil
}
