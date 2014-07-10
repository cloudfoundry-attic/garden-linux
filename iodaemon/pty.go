package main

import (
	"os"
	"syscall"
	"unsafe"
)

type ttySize struct {
	Rows   uint16
	Cols   uint16
	Xpixel uint16
	Ypixel uint16
}

func setWinSize(f *os.File, cols uint16, rows uint16) error {
	_, _, e := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(f.Fd()),
		uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&ttySize{rows, cols, 0, 0})),
		0, 0, 0,
	)

	if e != 0 {
		return syscall.ENOTTY
	}

	return nil
}
