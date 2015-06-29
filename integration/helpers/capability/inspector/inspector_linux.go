package inspector

import (
	"fmt"
	"syscall"
)

// CAP_SETUID
// Make arbitrary manipulations of process UIDs
// (setuid(2), setreuid(2), setresuid(2), setfsuid(2));
// make forged UID when passing socket credentials via UNIX domain sockets.
func ProbeSETUID() {
	trace := func(msg string) {
		fmt.Printf("CAP_SETUID: %s.\n", msg)
	}

	const NOBODY_UID = 65534
	if err := syscall.Setuid(NOBODY_UID); err != nil {
		trace(fmt.Sprintf("syscall.Setuid failed with error: %s", err))
	} else {
		trace("syscall.Setuid succeeded")
	}

	if err := syscall.Setreuid(NOBODY_UID, NOBODY_UID); err != nil {
		trace(fmt.Sprintf("syscall.Setreuid failed with error: %s", err))
	} else {
		trace("syscall.Setreuid succeeded")
	}

	if err := syscall.Setresuid(NOBODY_UID, NOBODY_UID, NOBODY_UID); err != nil {
		trace(fmt.Sprintf("syscall.Setresuid failed with error: %s", err))
	} else {
		trace("syscall.Setresuid succeeded")
	}

	if err := syscall.Setfsuid(NOBODY_UID); err != nil {
		trace(fmt.Sprintf("syscall.Setfsuid failed with error: %s", err))
	} else {
		trace("syscall.Setfsuid succeeded")
	}
}
