package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
)

func main() {
	socketPath := flag.String("socket", "./run/wshd.sock", "Path to socket")
	user := flag.String("user", "vcap", "User to change to")
	// ******************** TODO: implement old flags *****************
	dir := flag.String("dir", "/home/vcap", "Working directory for the running process")
	flag.String("env", "", "Environment variables to set for the command.")
	flag.String("pidfile", "", "File to save container-namespaced pid of spawned process to")
	flag.Bool("rsh", false, "RSH compatibility mode")
	// ******************** TODO: imlement old flags *****************

	flag.Parse()

	extraArgs := flag.Args()
	if len(extraArgs) == 0 {
		fmt.Fprintf(os.Stderr, "Command name not provided.")
		os.Exit(255)
	}

	processSpec := &garden.ProcessSpec{
		Path: extraArgs[0],
		Args: extraArgs[1:],
		Env:  []string{"HELLO=1"},
		Dir:  *dir,
		User: *user,
	}

	processIO := &garden.ProcessIO{
		Stdin:  os.Stdin,
		Stderr: os.Stderr,
		Stdout: os.Stdout,
	}

	connector := &unix_socket.Connector{
		SocketPath: *socketPath,
	}

	proc, err := container_daemon.NewProcess(connector, processSpec, processIO)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Starting process: %s", err)
		os.Exit(255)
	}

	exitCode, err := proc.Wait()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Waiting for process to complete: %s", err)
		os.Exit(255)
	}

	os.Exit(exitCode)
}
