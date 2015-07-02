package inspector

import (
	"fmt"
	"io"
	"io/ioutil"
	"log/syslog"
	"os"
	"os/exec"
	"syscall"
)

// #cgo LDFLAGS: -lrt
// #include <stdlib.h>
// #include <unistd.h>
// #include <stdio.h>
// #include <signal.h>
// #include <time.h>
//
// int probeWakeAlarm() {
//   timer_t timerid;
//   struct itimerspec its;
//
//   if (timer_create(CLOCK_BOOTTIME_ALARM, NULL, &timerid) == -1) {
//     return -1;
//   }
//
//   its.it_value.tv_sec = 866208142;
//   its.it_value.tv_nsec = 42;
//   its.it_interval.tv_sec = its.it_value.tv_sec;
//   its.it_interval.tv_nsec = its.it_value.tv_nsec;
//
//   if (timer_settime(timerid, 0, &its, NULL) == -1) {
//     return -2;
//   }
//   return 0;
// }
import "C"

// CAP_SETUID
// Make arbitrary manipulations of process UIDs
// (setuid(2), setreuid(2), setresuid(2), setfsuid(2));
// make forged UID when passing socket credentials via UNIX domain sockets.
func ProbeSETUID(uid, gid int) error {
	cmd := exec.Command("ls", "/")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}

	if err := cmd.Run(); err != nil {
		printErr("CAP_SETUID", "Failed to exec binary as %d:%d error: %s", uid, gid, err)
		return err
	} else {
		printInfo("CAP_SETUID", "Exec binary as %d:%d succeeded", uid, gid)
	}

	if err := syscall.Setreuid(uid, uid); err != nil {
		printErr("CAP_SETUID", "syscall.Setreuid for %d:%d failed with error: %s", uid, gid, err)
		return err
	} else {
		printInfo("CAP_SETUID", "syscall.Setreuid for %d:%d succeeded", uid, gid)
	}

	if err := syscall.Setresuid(uid, uid, uid); err != nil {
		printErr("CAP_SETUID", "syscall.Setresuid for %d:%d failed with error: %s", uid, gid, err)
		return err
	} else {
		printInfo("CAP_SETUID", "syscall.Setresuid for %d succeeded", uid)
	}

	if err := syscall.Setfsuid(uid); err != nil {
		printErr("CAP_SETUID", "syscall.Setfsuid for %d failed with error: %s", uid, err)
		return err
	} else {
		printInfo("CAP_SETUID", "syscall.Setfsuid for %d succeeded", uid)
	}

	return nil
}

// CAP_SETGID
// Make arbitrary manipulations of process GIDs and supplementary GID list;
// forge GID when passing socket credentials via UNIX domain sockets.
func ProbeSETGID(uid, gid int) error {
	cmd := exec.Command("ls", "/")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}

	if err := cmd.Run(); err != nil {
		printErr("CAP_SETGID", "Failed to exec binary as %d:%d error: %s", uid, gid, err)
		return err
	} else {
		printInfo("CAP_SETGID", "Exec binary as %d:%d succeeded", uid, gid)
	}

	return nil
}

func ProbeCHOWN(uid, gid int) error {
	file, err := ioutil.TempFile("", "")

	if err != nil {
		printErr("CAP_CHOWN", "Failed to create test file: %s", err)
		return err
	}

	if err := os.Chown(file.Name(), uid, gid); err != nil {
		printErr("CAP_CHOWN", "Failed to exec chown: %s", err)
		return err
	} else {
		printInfo("CAP_CHOWN", "Chown to %d:%d succeeded", uid, gid)
	}

	return nil
}

func ProbeSYSTIME() error {
	time := &syscall.Timeval{
		Sec:  866208142,
		Usec: 290944,
	}

	if err := syscall.Settimeofday(time); err != nil {
		printErr("CAP_SYSTIME", "syscall.Settimeofday failed with error: %s", err)
		return err
	} else {
		printInfo("CAP_SYSTIME", "syscall.Settimeofday succeeded")
	}

	return nil
}

func ProbeWAKEALARM() error {
	if rc := C.probeWakeAlarm(); rc != 0 {
		err := fmt.Errorf("Setting wake alarm: %d", rc)
		printErr("CAP_WAKE_ALARM", "Failed to set wake alarm: %s", err)
		return err
	} else {
		printInfo("CAP_WAKE_ALARM", "Setting wake alarm succeeded")
	}
	return nil
}

func ProbeSYSLOG() error {
	logger, err := syslog.New(syslog.LOG_INFO, "capability")
	if err != nil {
		printErr("CAP_SYSLOG", "Failed to create syslogger: %s", err)
		return err
	}

	if _, err := logger.Write([]byte("Capability tools is running")); err != nil {
		printErr("CAP_SYSLOG", "Writing to syslog failed with error: %s", err)
	} else {
		printInfo("CAP_SYSLOG", "Writing to syslog succeeded")
	}
	return nil
}

func printInfo(tag, msg string, args ...interface{}) {
	printMsg(os.Stdout, tag, msg, args...)
}

func printErr(tag, msg string, args ...interface{}) {
	printMsg(os.Stderr, tag, msg, args...)
}

func printMsg(std io.Writer, tag, msg string, args ...interface{}) {
	text := fmt.Sprintf(msg, args...)
	fmt.Fprintf(std, "%s: %s.\n", tag, text)
}
