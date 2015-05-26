package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
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

	programPath := getProgramPath(os.Args[2])
	if programPath == "" {
		fmt.Fprintf(os.Stderr, "ERROR: Program '%s' was not found in PATH.\n", os.Args[2])
		os.Exit(255)
	}

	err := syscall.Exec(programPath, os.Args[2:], os.Environ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: exec: %s\n", err)
		os.Exit(255)
	}
}

func getProgramPath(programName string) string {
	if _, err := os.Stat(programName); err == nil {
		return programName
	}

	// TODO(gl) Implement shell's behavior on program path searching
	// --
	//		[...] duplicate the actions of the shell in searching for an executable
	//		file if the **specified filename does not contain a slash (/)
	//		character.** The file is sought in the colon-separated list of
	//		directory pathnames specified in the PATH environment variable.
	//		If this variable isn't defined, the path list defaults to the current
	//		directory followed by the list of directories returned by
	//		`confstr(_CS_PATH)` [...]	(which) typically returns the value
	//		`"/bin:/usr/bin"`.
	// -- http://linux.die.net/man/3/execvp
	pathEnv := os.Getenv("PATH")
	for _, p := range strings.Split(pathEnv, ":") {
		candPath := path.Join(p, programName)
		if _, err := os.Stat(candPath); err == nil {
			return candPath
		}
	}

	return ""
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
