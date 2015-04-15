package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
)

func missing(flagName string) {
	fmt.Fprintf(os.Stderr, "%s is required\n", flagName)
	flag.Usage()
	os.Exit(1)
}

func main() {
	socketPath := flag.String("socket", "", "Path for the socket file")
	rootFsPath := flag.String("root", "", "Path for the root file system directory")
	flag.Parse()

	if *socketPath == "" {
		missing("--socket")
	}
	if *rootFsPath == "" {
		missing("--root")
	}

	sync := &containerizer.PipeSynchronizer{
		Reader: os.NewFile(uintptr(3), "/dev/a"),
		Writer: os.NewFile(uintptr(4), "/dev/d"),
	}

	containerizer := containerizer.Containerizer{
		RootFS: &system.RootFS{
			Root: *rootFsPath,
		},
		Daemon: &container_daemon.ContainerDaemon{
			SocketPath: *socketPath,
		},
		Waiter:    sync,
		Signaller: sync,
	}

	err := containerizer.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run containerizer: %s\n", err)
		os.Exit(2)
	}
}
