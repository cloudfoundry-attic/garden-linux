package inspector

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
)

const (
	STATUS_CODE_CAP_CHOWN = 200 + iota
	STATUS_CODE_CAP_SETUID
	STATUS_CODE_CAP_SETGID
	STATUS_CODE_CAP_SYS_TIME
)

type ProbeResult struct {
	StatusCode int
	Error      error
}

// CAP_SETUID
// Make arbitrary manipulations of process UIDs
// (setuid(2), setreuid(2), setresuid(2), setfsuid(2));
// make forged UID when passing socket credentials via UNIX domain sockets.
func ProbeSETUID(uid, gid int) ProbeResult {
	cmd := exec.Command("ls")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}

	if err := cmd.Run(); err != nil {
		trace("CAP_SETUID", "Failed to exec binary as %d:%d error: %s", uid, gid, err)
		return ProbeResult{1, err}
	} else {
		trace("CAP_SETUID", "Exec binary as %d:%d succeeded", uid, gid)
	}

	if err := syscall.Setreuid(uid, uid); err != nil {
		trace("CAP_SETUID", "syscall.Setreuid for %d:%d failed with error: %s", uid, gid, err)
		return ProbeResult{1, err}
	} else {
		trace("CAP_SETUID", "syscall.Setreuid for %d:%d succeeded", uid, gid)
	}

	if err := syscall.Setresuid(uid, uid, uid); err != nil {
		trace("CAP_SETUID", "syscall.Setresuid for %d:%d failed with error: %s", uid, gid, err)
		return ProbeResult{1, err}
	} else {
		trace("CAP_SETUID", "syscall.Setresuid for %d succeeded", uid)
	}

	if err := syscall.Setfsuid(uid); err != nil {
		trace("CAP_SETUID", "syscall.Setfsuid for %d failed with error: %s", uid, err)
		return ProbeResult{1, err}
	} else {
		trace("CAP_SETUID", "syscall.Setfsuid for %d succeeded", uid)
	}

	return ProbeResult{0, nil}
}

// CAP_SETGID
// Make arbitrary manipulations of process GIDs and supplementary GID list;
// forge GID when passing socket credentials via UNIX domain sockets.
func ProbeSETGID(uid, gid int) ProbeResult {
	cmd := exec.Command("ls")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}

	if err := cmd.Run(); err != nil {
		trace("CAP_SETGID", "Failed to exec binary as %d:%d error: %s", uid, gid, err)
		return ProbeResult{1, err}
	} else {
		trace("CAP_SETGID", "Exec binary as %d:%d succeeded", uid, gid)
	}

	return ProbeResult{0, nil}
}

func ProbeCHOWN(uid, gid int) ProbeResult {
	file, err := ioutil.TempFile("", "")

	if err != nil {
		trace("CAP_CHOWN", "Failed to create test file: %s", err)
		return ProbeResult{1, err}
	}

	// chown to nobody (do we need the uid of nobody?)
	// print success or failure message
	if err := os.Chown(file.Name(), uid, gid); err != nil {
		trace("CAP_CHOWN", "Failed to exec chown: %s", err)
		return ProbeResult{1, err}
	} else {
		trace("CAP_CHOWN", "Chown to %d:%d succeeded", uid, gid)
	}

	return ProbeResult{0, nil}
}

func ProbeSYSTIME() ProbeResult {
	time := &syscall.Timeval{
		Sec:  866208142,
		Usec: 290944,
	}

	if err := syscall.Settimeofday(time); err != nil {
		trace("CAP_SYSTIME", "syscall.Settimeofday failed with error: %s", err)
		return ProbeResult{STATUS_CODE_CAP_SYS_TIME, err}
	} else {
		trace("CAP_SYSTIME", "syscall.Settimeofday succeeded.")
	}

	return ProbeResult{0, nil}
}

func trace(tag, msg string, args ...interface{}) {
	text := fmt.Sprintf(msg, args...)
	fmt.Printf("%s: %s.\n", tag, text)
}
