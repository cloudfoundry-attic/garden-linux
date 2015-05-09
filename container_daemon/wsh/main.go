package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"io/ioutil"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	socketPath := flag.String("socket", "./run/wshd.sock", "Path to socket")
	user := flag.String("user", "vcap", "User to change to")
	dir := flag.String("dir", "/home/vcap", "Working directory for the running process")

	var envVars container_daemon.StringList
	flag.Var(&envVars, "env", "Environment variables to set for the command.")

	pidfile := flag.String("pidfile", "", "File to save container-namespaced pid of spawned process to")
	flag.Bool("rsh", false, "RSH compatibility mode")

	flag.Parse()

	extraArgs := flag.Args()
	if len(extraArgs) == 0 {
		fmt.Fprintf(os.Stderr, "command name not provided.")
		os.Exit(container_daemon.UnknownExitStatus)
	}

	var tty *garden.TTYSpec
	resize := make(chan os.Signal)
	if terminal.IsTerminal(syscall.Stdin) {
		tty = &garden.TTYSpec{}
		signal.Notify(resize, syscall.SIGWINCH)
	}

	process := &container_daemon.Process{
		Connector: &unix_socket.Connector{
			SocketPath: *socketPath,
		},

		Term: system.TermPkg{},

		SigwinchCh: resize,

		Spec: &garden.ProcessSpec{
			Path: extraArgs[0],
			Args: extraArgs[1:],
			Env:  envVars.List,
			Dir:  *dir,
			User: *user,
			TTY:  tty, // used as a boolean -- non-nil = attach pty
		},

		IO: &garden.ProcessIO{
			Stdin:  os.Stdin,
			Stderr: os.Stderr,
			Stdout: os.Stdout,
		},
	}

	err := process.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start process: %s", err)
		os.Exit(container_daemon.UnknownExitStatus)
	}

	defer process.Cleanup()

	if *pidFile != "" {
		pidString := fmt.Sprintf("%d\n", proc.Pid())
		err = ioutil.WriteFile(*pidFile, []byte(pidString), 0700)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Writing PID file: %s\n", err)
			os.Exit(container_daemon.UnknownExitStatus)
		}
	}

	exitCode, err := process.Wait()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wait for process: %s", err)
		os.Exit(container_daemon.UnknownExitStatus)
	}

	os.Exit(exitCode)
}
