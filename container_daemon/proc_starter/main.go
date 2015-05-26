package main

import (
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "ERROR: No arguments were provided!\n")
		os.Exit(255)
	}

	programPath := getProgramPath(os.Args[1])
	if programPath == "" {
		fmt.Fprintf(os.Stderr, "ERROR: Program '%s' was not found in PATH.\n", os.Args[1])
		os.Exit(255)
	}

	mgr := container_daemon.RlimitsManager{}
	envMap, _ := process.NewEnv(os.Environ())
	rlimits := mgr.DecodeLimits(envMap[container_daemon.RLimitsTag])
	mgr.Apply(rlimits)

	err := syscall.Exec(programPath, os.Args[1:], os.Environ())
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
