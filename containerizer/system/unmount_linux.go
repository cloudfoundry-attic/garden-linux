package system

import "syscall"

type Unmount struct {
	Dir string
}

func (u Unmount) Unmount() error {
	return syscall.Unmount(u.Dir, syscall.MNT_DETACH)
}
