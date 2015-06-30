package container_daemon

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/docker/docker/pkg/term"
)

const UnknownExitStatus = 255

type Process struct {
	Connector  Connector
	Term       Term
	SigwinchCh <-chan os.Signal
	Spec       *garden.ProcessSpec
	Pidfile    PidfileWriter
	IO         *garden.ProcessIO

	// assigned after Start() is called
	pid       int
	termState *term.State
	exitCode  <-chan int
	streaming *sync.WaitGroup
}

type PidfileWriter interface {
	Write(pid int) error
	Remove()
}

//go:generate counterfeiter -o fake_connector/FakeConnector.go . Connector
type Connector interface {
	Connect(msg interface{}) ([]unix_socket.Fd, int, error)
}

// wraps docker/docker/pkg/term for mockability
//go:generate counterfeiter -o fake_term/fake_term.go . Term
type Term interface {
	GetWinsize(fd uintptr) (*term.Winsize, error)
	SetWinsize(fd uintptr, size *term.Winsize) error

	SetRawTerminal(fd uintptr) (*term.State, error)
	RestoreTerminal(fd uintptr, state *term.State) error
}

func (p *Process) Start() error {
	fds, pid, err := p.Connector.Connect(p.Spec)
	if err != nil {
		return fmt.Errorf("container_daemon: connect to socket: %s", err)
	}

	if err := p.Pidfile.Write(pid); err != nil {
		return fmt.Errorf("container_daemon: write pidfile: %s", err)
	}

	p.streaming = &sync.WaitGroup{}

	if p.Spec.TTY != nil {
		p.setupPty(fds[0])
		fwdOverPty(fds[0], p.IO, p.streaming)
		p.exitCode = waitForExit(fds[1], p.streaming)
	} else {
		fwdNoninteractive(fds[0], fds[1], fds[2], p.IO, p.streaming)
		p.exitCode = waitForExit(fds[3], p.streaming)
	}

	return nil
}

func (p *Process) setupPty(ptyFd unix_socket.Fd) error {
	p.termState, _ = p.Term.SetRawTerminal(os.Stdin.Fd())

	go p.sigwinchLoop(ptyFd)
	return p.syncWindowSize(ptyFd)
}

func (p *Process) sigwinchLoop(ptyFd unix_socket.Fd) {
	for {
		<-p.SigwinchCh
		p.syncWindowSize(ptyFd)
	}
}

func (p *Process) syncWindowSize(ptyFd unix_socket.Fd) error {
	winsize, _ := p.Term.GetWinsize(os.Stdin.Fd())
	return p.Term.SetWinsize(ptyFd.Fd(), winsize)
}

func fwdOverPty(ptyFd io.ReadWriteCloser, processIO *garden.ProcessIO, streaming *sync.WaitGroup) {
	if processIO == nil {
		return
	}

	if processIO.Stdout != nil {
		streaming.Add(1)
		go func() {
			defer streaming.Done()
			io.Copy(processIO.Stdout, ptyFd)
		}()
	}

	if processIO.Stdin != nil {
		go io.Copy(ptyFd, processIO.Stdin)
	}
}

func fwdNoninteractive(stdinFd, stdoutFd, stderrFd io.ReadWriteCloser, processIO *garden.ProcessIO, streaming *sync.WaitGroup) {
	if processIO != nil && processIO.Stdin != nil {
		go copyAndClose(stdinFd, processIO.Stdin) // Ignore error
	}

	if processIO != nil && processIO.Stdout != nil {
		streaming.Add(1)
		go func() {
			defer streaming.Done()
			copyWithClose(processIO.Stdout, stdoutFd) // Ignore error
		}()
	}

	if processIO != nil && processIO.Stderr != nil {
		streaming.Add(1)
		go func() {
			defer streaming.Done()
			copyWithClose(processIO.Stderr, stderrFd) // Ignore error
		}()
	}
}

func copyAndClose(dst io.WriteCloser, src io.Reader) error {
	_, err := io.Copy(dst, src)
	dst.Close() // Ignore error
	return err
}

func copyWithClose(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	if rc, ok := src.(io.ReadCloser); ok {
		return rc.Close()
	}
	if wc, ok := dst.(io.WriteCloser); ok {
		return wc.Close()
	}
	return err
}

func (p *Process) Cleanup() {
	if p.termState != nil {
		p.Term.RestoreTerminal(os.Stdin.Fd(), p.termState)
	}
}

func (p *Process) Wait() (int, error) {
	defer p.Pidfile.Remove()

	doneStreaming := make(chan bool)
	go func() {
		p.streaming.Wait()
		doneStreaming <- true
	}()

	select {
	case <-doneStreaming:
	case <-time.After(100 * time.Millisecond):
		// allow a little time in case we're not quite done returning copied data
	}

	return <-p.exitCode, nil
}

func waitForExit(exitFd io.ReadWriteCloser, streaming *sync.WaitGroup) chan int {
	exitChan := make(chan int)
	go func(exitFd io.Reader, exitChan chan<- int, streaming *sync.WaitGroup) {
		b := make([]byte, 1)
		n, err := exitFd.Read(b)
		if n == 0 && err != nil {
			b[0] = UnknownExitStatus
		}

		exitChan <- int(b[0])
	}(exitFd, exitChan, streaming)

	return exitChan
}
