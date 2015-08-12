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
	"github.com/pivotal-golang/lager"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	socketPath := flag.String("socket", "./run/wshd.sock", "Path to socket")
	user := flag.String("user", "vcap", "User to change to")
	dir := flag.String("dir", "", "Working directory for the running process")
	logFile := flag.String("log", "", "file to log to")
	flag.Bool("rsh", false, "RSH compatibility mode")
	var envVars container_daemon.StringList
	flag.Var(&envVars, "env", "Environment variables to set for the command.")

	flag.Parse()

	logger := lager.NewLogger("container_daemon.wsh")
	if logFile != nil {
		log, _ := os.OpenFile(*logFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
		logger.RegisterSink(lager.NewWriterSink(log, lager.DEBUG))
	}

	extraArgs := flag.Args()
	if len(extraArgs) == 0 {
		// Default is to run a shell.
		extraArgs = []string{"/bin/sh"}
	}

	var tty *garden.TTYSpec
	resize := make(chan os.Signal, 1)
	if terminal.IsTerminal(syscall.Stdin) {
		tty = &garden.TTYSpec{}
		signal.Notify(resize, syscall.SIGWINCH)
	}

	i := 0 // number of fd after stdin, stdout and stderr
	// see ExtraFiles property in https://golang.org/pkg/os/exec/#Cmd
	signalReader := os.NewFile(uintptr(3+i), "extrafd")

	process := &container_daemon.Process{
		Connector: &unix_socket.Connector{
			SocketPath: *socketPath,
		},

		Term: container_daemon.TermPkg{},

		SigwinchCh: resize,

		SignalReader: signalReader,

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

		Logger: logger,
	}

	exitCode := container_daemon.UnknownExitStatus
	defer func() {
		process.Cleanup()
		os.Exit(exitCode)
	}()

	err := process.Start()
	if err != nil {
		logger.Error("start", err)
		return
	}

	exitCode, err = process.Wait()
	if err != nil {
		logger.Error("wait", err)
		return
	}
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
