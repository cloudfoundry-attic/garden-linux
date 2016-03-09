package system

import (
	"errors"
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

func (UserExecer) ExecAsUser(uid, gid int, workDir, programName string, args ...string) error {
	if _, _, errNo := syscall.RawSyscall(syscall.SYS_SETGID, uintptr(gid), 0, 0); errNo != 0 {
		return fmt.Errorf("system: setgid: %s", errNo.Error())
	}

	if err := syscall.Setgroups([]int{}); err != nil {
		return fmt.Errorf("system: setgroups: %s", err)
	}

	if _, _, errNo := syscall.RawSyscall(syscall.SYS_SETUID, uintptr(uid), 0, 0); errNo != 0 {
		return fmt.Errorf("system: setuid: %s", errNo.Error())
	}

	if workDir == "" {
		return errors.New("system: working directory is not provided.")
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("system: %s", err)
	}

	if err := os.Chdir(workDir); err != nil {
		return fmt.Errorf("system: invalid working directory: %s", workDir)
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
