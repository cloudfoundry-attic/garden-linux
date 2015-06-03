package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "ERROR: No arguments were provided!\n")
		os.Exit(255)
	}

	mgr := &container_daemon.RlimitsManager{}
	rlimits := mgr.DecodeLimits(decodeRLimitsArg(os.Args[1]))
	mgr.Apply(rlimits)

	programPath, err := exec.LookPath(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Program '%s' was not found in $PATH: %s\n", os.Args[2], err)
		os.Exit(255)
	}

	err = syscall.Exec(programPath, os.Args[2:], os.Environ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: exec: %s\n", err)
		os.Exit(255)
	}
}

func decodeRLimitsArg(rlimitsArg string) string {
	var rlimits string
	count, err := fmt.Sscanf(rlimitsArg, container_daemon.RLimitsTag+"=%s", &rlimits)

	if count != 1 || err != nil {
		if err == io.EOF {
			return ""
		}
		fmt.Fprintf(os.Stderr, "ERROR: invalid rlimits argument: %s\n", rlimitsArg)
		os.Exit(255)
	}

	return rlimits
}
