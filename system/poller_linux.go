package system

import (
	"fmt"
	"syscall"
	"unsafe"
)

const POLLIN = 0x1
const timeout = 4000000000

func NewPoller(fds []uintptr) *Poller {
	p := Poller{}
	p.fds = make([]pollfd, len(fds))

	for i, fd := range fds {
		p.fds[i] = pollfd{
			fd:     int32(fd),
			events: POLLIN,
		}
	}
	return &p

}

// Polls the file descriptors and returns when at least one of them is ready for reading.
//
// NOTE: If a file descriptor is closed it is always ready for reading.
func (p *Poller) Poll() error {
	for {
		numReadyPtr, _, errno := syscall.Syscall(syscall.SYS_POLL, uintptr(unsafe.Pointer(&(p.fds[0]))),
			uintptr(len(p.fds)), uintptr(timeout))

		numReady := int(numReadyPtr)
		if numReady < 0 && errno != syscall.EINTR {
			return fmt.Errorf("system: poll failed: %s", errno.Error())
		}

		if numReady > 0 {
			return nil
		}
	}
}
