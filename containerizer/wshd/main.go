package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
)

func missing(flagName string) {
	fmt.Fprintf(os.Stderr, "%s is required\n", flagName)
	flag.Usage()
	os.Exit(1)
}

func main() {
	socketPath := flag.String("socket", "/tmp/wshd.sock", "Path for the socket file")
	rootFsPath := flag.String("root", "", "Path for the root file system directory")

	// ******************** TODO: remove old flags *****************
	flag.String("lib", "", "")
	flag.String("userns", "", "")
	flag.String("run", "", "")
	flag.String("title", "", "")
	// ******************** TODO: remove old flags *****************

	flag.Parse()

	if *rootFsPath == "" {
		missing("--root")
	}

	binPath, _ := filepath.Abs(filepath.Dir(os.Args[0]))

	a, b, _ := os.Pipe()
	c, d, _ := os.Pipe()
	sync := &containerizer.PipeSynchronizer{
		Reader: c,
		Writer: b,
	}

	cz := containerizer.Containerizer{
		InitBinPath: path.Join(binPath, "initd"),
		InitArgs:    []string{"--socket", *socketPath, "--root", *rootFsPath},
		Execer: &system.Execer{
			CommandRunner: linux_command_runner.New(),
			ExtraFiles:    []*os.File{a, d},
		},
		Signaller: sync,
		Waiter:    sync,
	}

	err := cz.Create()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create container: %s", err)
		os.Exit(2)
	}
}
