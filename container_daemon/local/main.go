package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/docker/docker/pkg/term"
	"github.com/kr/pty"
	"github.com/pivotal-golang/lager"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	tmp, err := ioutil.TempDir("", "local_daemon")
	if err != nil {
		panic(err)
	}

	socketPath := path.Join(tmp, "local_daemon.sock")

	server(socketPath)
	client(socketPath)
}

func client(socketPath string) {
	var envVars container_daemon.StringList
	user := flag.String("user", "vcap", "User to change to")
	dir := flag.String("dir", "/home/vcap", "Working directory for the running process")
	flag.Var(&envVars, "env", "Environment variables to set for the command.")
	flag.Parse()

	extraArgs := flag.Args()
	if len(extraArgs) == 0 {
		fmt.Fprintf(os.Stderr, "Command name not provided.")
		os.Exit(container_daemon.UnknownExitStatus)
	}

	var tty *garden.TTYSpec
	if terminal.IsTerminal(syscall.Stdin) {
		tty = &garden.TTYSpec{}
		state, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			fmt.Fprintf(os.Stderr, "set up stdin as raw terminal: %s", err)
		}

		defer term.RestoreTerminal(os.Stdin.Fd(), state)
	}

	spec := &garden.ProcessSpec{
		Path: extraArgs[0],
		Args: extraArgs[1:],
		Env:  envVars.List,
		Dir:  *dir,
		User: *user,
		TTY:  tty,
	}

	io := &garden.ProcessIO{
		Stdin:  os.Stdin,
		Stderr: os.Stderr,
		Stdout: os.Stdout,
	}

	process, err := container_daemon.NewProcess(
		&unix_socket.Connector{socketPath},
		spec,
		io,
	)

	if err != nil {
		panic(err)
	}

	process.Wait()
}

func server(socketPath string) {
	log := lager.NewLogger("local_daemon")

	reaper := system.StartReaper(log)

	daemon := &container_daemon.ContainerDaemon{
		CmdPreparer: &container_daemon.ProcessSpecPreparer{
			Users: golangUser{},
		},
		Spawner: &container_daemon.Spawn{
			Runner: reaper,
			PTY:    KrPty,
		},
	}

	listener, err := unix_socket.NewListenerFromPath(socketPath)
	if err != nil {
		panic(err)
	}

	go daemon.Run(listener)
}

type krPty int

var KrPty krPty = 0

func (krPty) Open() (*os.File, *os.File, error) {
	return pty.Open()
}

type golangUser struct {
}

func (golangUser) Lookup(name string) (*user.User, error) {
	return user.Lookup(name)
}
