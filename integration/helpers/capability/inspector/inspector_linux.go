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
)

type ProbeResult struct {
	StatusCode int
	Error      error
}

// CAP_SETUID
// Make arbitrary manipulations of process UIDs
// (setuid(2), setreuid(2), setresuid(2), setfsuid(2));
// make forged UID when passing socket credentials via UNIX domain sockets.
func ProbeSETUID(uid, gid int) {
	ProbeSETGID(uid, gid)

	if err := syscall.Setreuid(uid, uid); err != nil {
		trace("CAP_SETUID", "syscall.Setreuid for %d:%d failed with error: %s", uid, gid, err)
	} else {
		trace("CAP_SETUID", "syscall.Setreuid for %d:%d succeeded", uid, gid)
	}

	if err := syscall.Setresuid(uid, uid, uid); err != nil {
		trace("CAP_SETUID", "syscall.Setresuid for %d:%d failed with error: %s", uid, gid, err)
	} else {
		trace("CAP_SETUID", "syscall.Setresuid for %d succeeded", uid)
	}

	if err := syscall.Setfsuid(uid); err != nil {
		trace("CAP_SETUID", "syscall.Setfsuid for %d failed with error: %s", uid, err)
	} else {
		trace("CAP_SETUID", "syscall.Setfsuid for %d succeeded", uid)
	}
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
		return ProbeResult{STATUS_CODE_CAP_SETGID, err}
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
		return ProbeResult{STATUS_CODE_CAP_CHOWN, err}
	} else {
		trace("CAP_CHOWN", "Chown to %d:%d succeeded", uid, gid)
	}

	return ProbeResult{0, nil}
}

func trace(tag, msg string, args ...interface{}) {
	text := fmt.Sprintf(msg, args...)
	fmt.Printf("%s: %s.\n", tag, text)
}
