package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"

	"flag"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/system"
)

// proc_starter starts a user process with the correct rlimits and after
// closing any open FDs.
func main() {
	runtime.LockOSThread()

	rlimits := flag.String("rlimits", "", "encoded rlimits")
	dropCapabilities := flag.Bool("dropCapabilities", true, "drop capabilties before starting process")
	uid := flag.Int("uid", -1, "user id to run the process as")
	gid := flag.Int("gid", -1, "group id to run the process as")
	flag.Parse()

	closeFds()

	mgr := &container_daemon.RlimitsManager{}
	must(mgr.Apply(mgr.DecodeLimits(*rlimits)))

	args := flag.Args()

	if *dropCapabilities {
		caps := &system.ProcessCapabilities{Pid: os.Getpid()}
		must(caps.Limit())
	}

	runAsUser(*uid, *gid, args[0], args)
}

func runAsUser(uid, gid int, programName string, args []string) {
	if _, _, errNo := syscall.RawSyscall(syscall.SYS_SETGID, uintptr(gid), 0, 0); errNo != 0 {
		fmt.Fprintf(os.Stderr, "setgid: %s", errNo.Error())
		os.Exit(255)
	}
	if _, _, errNo := syscall.RawSyscall(syscall.SYS_SETUID, uintptr(uid), 0, 0); errNo != 0 {
		fmt.Fprintf(os.Stderr, "setuid: %s", errNo.Error())
		os.Exit(255)
	}

	programPath, err := exec.LookPath(programName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Program '%s' was not found in $PATH: %s\n", programName, err)
		os.Exit(255)
	}

	if err := syscall.Exec(programPath, args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: exec: %s\n", err)
		os.Exit(255)
	}
}

func closeFds() {
	fds, err := ioutil.ReadDir("/proc/self/fd")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: read /proc/self/fd: %s", err)
		os.Exit(255)
	}

	for _, fd := range fds {
		if fd.IsDir() {
			continue
		}

		fdI, err := strconv.Atoi(fd.Name())
		if err != nil {
			panic(err) // cant happen
		}

		if fdI <= 2 {
			continue
		}

		syscall.CloseOnExec(fdI)
	}
}

var must = mustNot

func mustNot(err error) {
	if err != nil {
		panic(err)
	}
}
