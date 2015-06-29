package inspector

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
)

const (
	NOBODY_UID = 65534
	NOBODY_GID = 65534
)

// CAP_SETUID
// Make arbitrary manipulations of process UIDs
// (setuid(2), setreuid(2), setresuid(2), setfsuid(2));
// make forged UID when passing socket credentials via UNIX domain sockets.
func ProbeSETUID() {
	trace := func(msg string) {
		fmt.Printf("CAP_SETUID: %s.\n", msg)
	}

	ProbeSETGID()

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

// CAP_SETGID
// Make arbitrary manipulations of process GIDs and supplementary GID list;
// forge GID when passing socket credentials via UNIX domain sockets.
func ProbeSETGID() {
	trace := func(msg string) {
		fmt.Printf("CAP_SETGID | CAP_SETUID: %s.\n", msg)
	}

	cmd := exec.Command("ls")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid: NOBODY_UID,
		Gid: NOBODY_GID,
	}

	// Setuid is not allowed in GO, we should spawn a process with UID and GID
	// if err := syscall.Setuid(NOBODY_UID); err != nil {
	// if err := syscall.Setgid(NOBODY_GID); err != nil {
	if err := cmd.Run(); err != nil {
		trace(fmt.Sprintf("Failed to exec binary as %d:%d error: %s", NOBODY_UID, NOBODY_GID, err))
	} else {
		trace(fmt.Sprintf("Exec binary as %d:%d succeeded", NOBODY_UID, NOBODY_GID))
	}
}

func ProbeCHOWN() {
	const CAP_CHOWN = "CAP_CHOWN"
	// create temp file. If it fails, break put and print an error message.. No fallback right now.
	file, err := ioutil.TempFile("", "")

	if err != nil {
		trace(CAP_CHOWN, fmt.Sprintf("Failed to create test file: %s", err))
		return
	}

	// chown to nobody (do we need the uid of nobody?)
	// print success or failure message
	if err := os.Chown(file.Name(), NOBODY_UID, NOBODY_GID); err != nil {
		trace(CAP_CHOWN, fmt.Sprintf("Failed to exec chown: %s", err))
	} else {
		trace(CAP_CHOWN, fmt.Sprintf("Chown to %d:%d succeeded", NOBODY_UID, NOBODY_GID))
	}
}

func trace(tag, msg string) {
	fmt.Printf("%s: %s.\n", tag, msg)
}
