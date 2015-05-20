package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
		// Default is to run a shell.
		extraArgs = []string{"/bin/sh"}
	}

	var tty *garden.TTYSpec
	resize := make(chan os.Signal)
	if terminal.IsTerminal(syscall.Stdin) {
		tty = &garden.TTYSpec{}
		signal.Notify(resize, syscall.SIGWINCH)
	}

	var pidfileWriter container_daemon.PidfileWriter = container_daemon.NoPidfile{}
	if *pidfile != "" {
		pidfileWriter = container_daemon.Pidfile{
			Path: *pidfile,
		}
	}

	process := &container_daemon.Process{
		Connector: &unix_socket.Connector{
			SocketPath: *socketPath,
		},

		Term: system.TermPkg{},

		Pidfile: pidfileWriter,

		SigwinchCh: resize,

		Spec: &garden.ProcessSpec{
			Path:   extraArgs[0],
			Args:   extraArgs[1:],
			Env:    envVars.List,
			Dir:    *dir,
			User:   *user,
			TTY:    tty, // used as a boolean -- non-nil = attach pty
			Limits: getRlimits(),
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

	exitCode, err := process.Wait()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wait for process: %s", err)
		os.Exit(container_daemon.UnknownExitStatus)
	}

	os.Exit(exitCode)
}

func getRLimitFromEnv(envVar string) *uint64 {
	strVal := os.Getenv(envVar)
	if strVal == "" {
		return nil
	}

	var val uint64
	fmt.Sscanf(strVal, "%d", &val)
	return &val
}

func getRlimits() garden.ResourceLimits {
	return garden.ResourceLimits{
		As:         getRLimitFromEnv("RLIMIT_AS"),
		Core:       getRLimitFromEnv("RLIMIT_CORE"),
		Cpu:        getRLimitFromEnv("RLIMIT_CPU"),
		Data:       getRLimitFromEnv("RLIMIT_DATA"),
		Fsize:      getRLimitFromEnv("RLIMIT_FSIZE"),
		Locks:      getRLimitFromEnv("RLIMIT_LOCKS"),
		Memlock:    getRLimitFromEnv("RLIMIT_MEMLOCK"),
		Msgqueue:   getRLimitFromEnv("RLIMIT_MSGQUEUE"),
		Nice:       getRLimitFromEnv("RLIMIT_NICE"),
		Nofile:     getRLimitFromEnv("RLIMIT_NOFILE"),
		Nproc:      getRLimitFromEnv("RLIMIT_NPROC"),
		Rss:        getRLimitFromEnv("RLIMIT_RSS"),
		Rtprio:     getRLimitFromEnv("RLIMIT_RTPRIO"),
		Sigpending: getRLimitFromEnv("RLIMIT_SIGPENDING"),
		Stack:      getRLimitFromEnv("RLIMIT_STACK"),
	}
}
