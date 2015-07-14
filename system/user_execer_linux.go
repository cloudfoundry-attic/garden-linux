package system

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

func init() {
	runtime.LockOSThread()
}

type UserExecer struct{}

func (UserExecer) ExecAsUser(uid, gid int, programName string, args ...string) error {
	if _, _, errNo := syscall.RawSyscall(syscall.SYS_SETGID, uintptr(gid), 0, 0); errNo != 0 {
		return fmt.Errorf("system: setgid: %s", errNo.Error())
	}
	if _, _, errNo := syscall.RawSyscall(syscall.SYS_SETUID, uintptr(uid), 0, 0); errNo != 0 {
		return fmt.Errorf("system: setuid: %s", errNo.Error())
	}

	programPath, err := exec.LookPath(programName)
	if err != nil {
		return fmt.Errorf("system: program '%s' was not found in $PATH: %s", programName, err)
	}

	if err := syscall.Exec(programPath, append([]string{programName}, args...), os.Environ()); err != nil {
		return fmt.Errorf("system: exec of %s: %s", programName, err)
	}
	return nil
}
