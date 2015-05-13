package container_daemon

import (
	"fmt"
	"io"
	"os"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/docker/docker/pkg/term"
)

const UnknownExitStatus = 255

type Process struct {
	Connector  Connector
	Term       system.Term
	SigwinchCh <-chan os.Signal
	Spec       *garden.ProcessSpec
	Pidfile    PidfileWriter
	IO         *garden.ProcessIO

	// assigned after Start() is called
	pid      int
	state    *term.State
	exitCode <-chan int
}

type PidfileWriter interface {
	Write(pid int) error
	Remove()
}

//go:generate counterfeiter -o fake_connector/FakeConnector.go . Connector
type Connector interface {
	Connect(msg interface{}) ([]unix_socket.Fd, int, error)
}

func (p *Process) Start() error {
	fds, pid, err := p.Connector.Connect(p.Spec)
	if err != nil {
		return fmt.Errorf("container_daemon: connect to socket: %s", err)
	}

	if err := p.Pidfile.Write(pid); err != nil {
		return fmt.Errorf("container_daemon: write pidfile: %s", err)
	}

	if p.Spec.TTY != nil {
		p.setupPty(fds[0])
		fwdOverPty(fds[0], p.IO)
		p.exitCode = waitForExit(fds[1])
	} else {
		fwdNoninteractive(fds[0], fds[1], fds[2], p.IO)
		p.exitCode = waitForExit(fds[3])
	}

	return nil
}

func (p *Process) setupPty(ptyFd unix_socket.Fd) error {
	p.state, _ = p.Term.SetRawTerminal(os.Stdin.Fd())

	go p.sigwinchLoop(ptyFd)
	return p.syncWindowSize(ptyFd)
}

func (p *Process) sigwinchLoop(ptyFd unix_socket.Fd) {
	for {
		select {
		case <-p.SigwinchCh:
			p.syncWindowSize(ptyFd)
		}
	}
}

func (p *Process) syncWindowSize(ptyFd unix_socket.Fd) error {
	winsize, _ := p.Term.GetWinsize(os.Stdin.Fd())
	return p.Term.SetWinsize(ptyFd.Fd(), winsize)
}

func fwdOverPty(ptyFd io.ReadWriteCloser, processIO *garden.ProcessIO) {
	if processIO == nil {
		return
	}

	if processIO.Stdout != nil {
		go io.Copy(processIO.Stdout, ptyFd)
	}

	if processIO.Stdin != nil {
		go io.Copy(ptyFd, processIO.Stdin)
	}
}

func fwdNoninteractive(stdinFd, stdoutFd, stderrFd io.ReadWriteCloser, processIO *garden.ProcessIO) {
	if processIO != nil && processIO.Stdin != nil {
		go copyAndClose(stdinFd, processIO.Stdin) // Ignore error
	}

	if processIO != nil && processIO.Stdout != nil {
		go io.Copy(processIO.Stdout, stdoutFd) // Ignore error
	}

	if processIO != nil && processIO.Stderr != nil {
		go io.Copy(processIO.Stderr, stderrFd) // Ignore error
	}
}

func copyAndClose(dst io.WriteCloser, src io.Reader) error {
	_, err := io.Copy(dst, src)
	dst.Close() // Ignore error
	return err
}

func (p *Process) Cleanup() {
	if p.state != nil {
		p.Term.RestoreTerminal(os.Stdin.Fd(), p.state)
	}
}

func (p *Process) Wait() (int, error) {
	defer p.Pidfile.Remove()

	return <-p.exitCode, nil
}

func waitForExit(exitFd io.ReadWriteCloser) chan int {
	exitChan := make(chan int)
	go func(exitFd io.Reader, exitChan chan<- int) {
		b := make([]byte, 1)
		n, err := exitFd.Read(b)
		if n == 0 && err != nil {
			b[0] = UnknownExitStatus
		}

		exitChan <- int(b[0])
	}(exitFd, exitChan)

	return exitChan
}
