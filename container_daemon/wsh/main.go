package main

import (
	"flag"
	"fmt"
	"os"

	"io/ioutil"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
)

func main() {
	var envVars container_daemon.StringList
	socketPath := flag.String("socket", "./run/wshd.sock", "Path to socket")
	user := flag.String("user", "vcap", "User to change to")
	pidFile := flag.String("pidfile", "", "File to save container-namespaced pid of spawned process to")
	// ******************** TODO: implement old flags *****************
	dir := flag.String("dir", "/home/vcap", "Working directory for the running process")
	flag.Var(&envVars, "env", "Environment variables to set for the command.")
	flag.Bool("rsh", false, "RSH compatibility mode")
	// ******************** TODO: imlement old flags *****************

	flag.Parse()

	extraArgs := flag.Args()
	if len(extraArgs) == 0 {
		fmt.Fprintf(os.Stderr, "Command name not provided.")
		os.Exit(container_daemon.UnknownExitStatus)
	}

	processSpec := &garden.ProcessSpec{
		Path: extraArgs[0],
		Args: extraArgs[1:],
		Env:  envVars.List,
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
		fmt.Fprintf(os.Stderr, "Starting process: %s\n", err)
		os.Exit(container_daemon.UnknownExitStatus)
	}

	if *pidFile != "" {
		pidString := fmt.Sprintf("%d\n", proc.Pid())
		err = ioutil.WriteFile(*pidFile, []byte(pidString), 0700)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Writing PID file: %s\n", err)
			os.Exit(container_daemon.UnknownExitStatus)
		}
	}

	exitCode, err := proc.Wait()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Waiting for process to complete: %s\n", err)
		os.Exit(container_daemon.UnknownExitStatus)
	}

	os.Exit(exitCode)
}
