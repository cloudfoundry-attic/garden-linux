package container_daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/docker/docker/pkg/term"
	"github.com/pivotal-golang/lager"
)

const UnknownExitStatus = 255

type Process struct {
	Connector  Connector
	Term       Term
	SigwinchCh <-chan os.Signal
	SigtermCh  <-chan os.Signal
	Spec       *garden.ProcessSpec
	IO         *garden.ProcessIO

	// assigned after Start() is called
	pid       int
	termState *term.State
	exitCode  <-chan int
	streaming *sync.WaitGroup

	logger lager.Logger
}

//go:generate counterfeiter -o fake_connector/FakeConnector.go . Connector
type Connector interface {
	Connect(msg *RequestMessage) (*ResponseMessage, error)
}

// wraps docker/docker/pkg/term for mockability
//go:generate counterfeiter -o fake_term/fake_term.go . Term
type Term interface {
	GetWinsize(fd uintptr) (*term.Winsize, error)
	SetWinsize(fd uintptr, size *term.Winsize) error

	SetRawTerminal(fd uintptr) (*term.State, error)
	RestoreTerminal(fd uintptr, state *term.State) error
}

func (p *Process) Signal(signal os.Signal) error {
	if signal == syscall.SIGUSR1 {
		signal = syscall.SIGKILL
	}

	spec := &SignalSpec{
		Pid:    p.pid,
		Signal: signal.(syscall.Signal),
	}

	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("container_daemon: marshal signal spec json: %s", err)
	}

	request := &RequestMessage{
		Type: SignalRequest,
		Data: data,
	}

	if _, err := p.Connector.Connect(request); err != nil {
		return fmt.Errorf("container_daemon: connect to some socket: %s", err)
	}

	return nil
}

func (p *Process) Start() error {
	p.logger = lager.NewLogger("container_daemon.Process")

	data, err := json.Marshal(p.Spec)
	if err != nil {
		return fmt.Errorf("container_daemon: marshal process spec json: %s", err)
	}

	request := &RequestMessage{
		Type: ProcessRequest,
		Data: data,
	}

	response, err := p.Connector.Connect(request)
	if err != nil {
		return fmt.Errorf("container_daemon: connect to socket: %s", err)
	}

	p.pid = response.Pid

	go p.sigtermLoop()
	p.streaming = &sync.WaitGroup{}

	if p.Spec.TTY != nil {
		p.setupPty(response.Files[0])
		p.fwdOverPty(response.Files[0])
		p.exitCode = p.exitWaitChannel(response.Files[1])
	} else {
		p.fwdNoninteractive(response.Files[0], response.Files[1], response.Files[2])
		p.exitCode = p.exitWaitChannel(response.Files[3])
	}

	return nil
}

func (p *Process) setupPty(ptyFd StreamingFile) error {
	p.termState, _ = p.Term.SetRawTerminal(os.Stdin.Fd())

	go p.sigwinchLoop(ptyFd)
	return p.syncWindowSize(ptyFd)
}

func (p *Process) sigtermLoop() {
	for {
		signal := <-p.SigtermCh
		p.Signal(signal)
	}
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
	if p.IO != nil && p.IO.Stdout != nil {
		p.stream(p.IO.Stdout, ptyFd)
	}

	if p.IO != nil && p.IO.Stdin != nil {
		go io.Copy(ptyFd, p.IO.Stdin)
	}
}

func (p *Process) fwdNoninteractive(stdinFd, stdoutFd, stderrFd StreamingFile) {
	if p.IO != nil && p.IO.Stdin != nil {
		go func() {
			io.Copy(stdinFd, p.IO.Stdin)
			stdinFd.Close()
		}()
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
		io.Copy(dst, src)
		p.streaming.Done()
	}()
}

func (p *Process) Cleanup() {
	if p.termState != nil {
		p.Term.RestoreTerminal(os.Stdin.Fd(), p.termState)
	}
}

func (p *Process) exitWaitChannel(exitFd io.ReadWriteCloser) chan int {
	exitChan := make(chan int)
	go func(exitFd io.Reader, exitChan chan<- int, streaming *sync.WaitGroup) {
		b := make([]byte, 1)
		n, err := exitFd.Read(b)
		if n == 0 && err != nil {
			b[0] = UnknownExitStatus
		}

		exitChan <- int(b[0])
	}(exitFd, exitChan, p.streaming)

	return exitChan
}

func (p *Process) Wait() (int, error) {
	exit := <-p.exitCode
	p.waitForStreamingToComplete()
	return exit, nil
}

func (p *Process) waitForStreamingToComplete() {
	doneStreaming := make(chan bool)
	go func() {
		p.streaming.Wait()
		doneStreaming <- true
	}()

	select {
	case <-doneStreaming:
	case <-time.After(200 * time.Millisecond):
		// allow a little time in case we're not quite done returning copied data
	}
}
