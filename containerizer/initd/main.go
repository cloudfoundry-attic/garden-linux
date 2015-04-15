package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
)

func missing(flagName string) {
	fmt.Fprintf(os.Stderr, "%s is required\n", flagName)
	flag.Usage()
	os.Exit(1)
}

func main() {
	socketPath := flag.String("socket", "", "Path for the socket file")
	flag.Parse()

	if *socketPath == "" {
		missing("--socket")
	}

	daemon := container_daemon.ContainerDaemon{
		SocketPath: *socketPath,
	}

	containerizer := containerizer.Containerizer{
		Daemon: daemon,
	}

	if err := daemon.Init(); err != nil {
		panic(fmt.Sprintf("Failed to initialize daemon: %v", err))
	}

	containerizer.Child()
}
