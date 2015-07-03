package container_daemon

import (
	"fmt"
	"io"
	"os"
	"sync"

	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/docker/docker/pkg/term"
	"github.com/pivotal-golang/lager"
)

const UnknownExitStatus = 255

type StreamingFile interface {
	io.ReadWriteCloser
	Fd() uintptr
}

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

	logger lager.Logger
}

type PidfileWriter interface {
	Write(pid int) error
	Remove()
}

//go:generate counterfeiter -o fake_connector/FakeConnector.go . Connector
type Connector interface {
	Connect(msg interface{}) ([]StreamingFile, int, error)
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
	p.logger = lager.NewLogger("container_daemon.Process")

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
		p.fwdOverPty(fds[0])
		p.exitCode = p.exitWaitChannel(fds[1])
	} else {
		p.fwdNoninteractive(fds[0], fds[1], fds[2])
		p.exitCode = p.exitWaitChannel(fds[3])
	}

	return nil
}

func (p *Process) setupPty(ptyFd StreamingFile) error {
	p.termState, _ = p.Term.SetRawTerminal(os.Stdin.Fd())

	go p.sigwinchLoop(ptyFd)
	return p.syncWindowSize(ptyFd)
}

func (p *Process) sigwinchLoop(ptyFd StreamingFile) {
	for {
		<-p.SigwinchCh
		p.syncWindowSize(ptyFd)
	}
}

func (p *Process) syncWindowSize(ptyFd StreamingFile) error {
	winsize, _ := p.Term.GetWinsize(os.Stdin.Fd())
	return p.Term.SetWinsize(ptyFd.Fd(), winsize)
}

func (p *Process) fwdOverPty(ptyFd StreamingFile) {
	if p.IO == nil {
		return
	}

	if p.IO.Stdout != nil {
		p.streamButDontClose(p.IO.Stdout, ptyFd)
	}

	if p.IO.Stdin != nil {
		go io.Copy(ptyFd, p.IO.Stdin)
	}
}

func (p *Process) fwdNoninteractive(stdinFd, stdoutFd, stderrFd StreamingFile) {
	if p.IO != nil && p.IO.Stdin != nil {
		go copyAndClose(stdinFd, p.IO.Stdin) // Ignore error
	}

	if p.IO != nil && p.IO.Stdout != nil {
		p.stream(p.IO.Stdout, stdoutFd)
	}

	if p.IO != nil && p.IO.Stderr != nil {
		p.stream(p.IO.Stderr, stderrFd)
	}
}

func (p *Process) stream(dst io.Writer, src StreamingFile) {
	p.streaming.Add(1)
	go func() {
		defer p.streaming.Done()
		copyWithClose(dst, src)
	}()
}

func (p *Process) streamButDontClose(dst io.Writer, src StreamingFile) {
	p.streaming.Add(1)
	go func() {
		defer p.streaming.Done()
		io.Copy(dst, src)
	}()
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

func copyAndClose(dst io.WriteCloser, src io.Reader) error {
	_, err := io.Copy(dst, src)
	dst.Close() // Ignore error
	return err
}

func (p *Process) Cleanup() {
	if p.termState != nil {
		p.Term.RestoreTerminal(os.Stdin.Fd(), p.termState)
	}
}

func (p *Process) Wait() (int, error) {
	defer p.Pidfile.Remove()

	return <-p.exitCode, nil
}

func (p *Process) exitWaitChannel(exitFd io.ReadWriteCloser) chan int {
	exitChan := make(chan int)
	go func(exitFd io.Reader, exitChan chan<- int, streaming *sync.WaitGroup) {
		b := make([]byte, 1)
		n, err := exitFd.Read(b)
		if n == 0 && err != nil {
			b[0] = UnknownExitStatus
		}

		doneStreaming := make(chan struct{})
		go func(ch chan<- struct{}) {
			streaming.Wait()
			ch <- struct{}{}
		}(doneStreaming)

		select {
		case <-doneStreaming:
		case <-time.After(time.Millisecond * 100):
		}

		exitChan <- int(b[0])
	}(exitFd, exitChan, p.streaming)

	return exitChan
}
