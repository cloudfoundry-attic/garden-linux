package process_tracker

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sync"
	"syscall"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry/gunk/command_runner"
)

type Process struct {
	ID uint32

	containerPath string
	runner        command_runner.CommandRunner

	waitingLinks   *sync.Mutex
	completionLock *sync.Mutex
	runningLink    *sync.Once
	link           *exec.Cmd

	linked   chan struct{}
	unlinked <-chan struct{}

	streams     []chan warden.ProcessStream
	streamsLock *sync.RWMutex

	completed bool

	exitStatus uint32
	stdout     *namedStream
	stderr     *namedStream
}

func NewProcess(
	id uint32,
	containerPath string,
	runner command_runner.CommandRunner,
) *Process {
	unlinked := make(chan struct{}, 1)
	unlinked <- struct{}{}

	p := &Process{
		ID: id,

		containerPath: containerPath,
		runner:        runner,

		streamsLock: &sync.RWMutex{},

		waitingLinks:   &sync.Mutex{},
		runningLink:    &sync.Once{},
		completionLock: &sync.Mutex{},
		linked:         make(chan struct{}),
		unlinked:       unlinked,
	}

	p.stdout = newNamedStream(p, warden.ProcessStreamSourceStdout)
	p.stderr = newNamedStream(p, warden.ProcessStreamSourceStderr)

	return p
}

func (p *Process) Spawn(cmd *exec.Cmd) (ready, active chan error) {
	ready = make(chan error, 1)
	active = make(chan error, 1)

	spawnPath := path.Join(p.containerPath, "bin", "iomux-spawn")
	processDir := path.Join(p.containerPath, "processes", fmt.Sprintf("%d", p.ID))

	mkdir := &exec.Cmd{
		Path: "mkdir",
		Args: []string{"-p", processDir},
	}

	err := p.runner.Run(mkdir)
	if err != nil {
		ready <- err
		return
	}

	spawn := &exec.Cmd{
		Path:  "bash",
		Stdin: cmd.Stdin,
	}

	spawn.Args = append([]string{"-c", "cat | " + spawnPath + ` "$@" &`, spawnPath, processDir}, cmd.Path)
	spawn.Args = append(spawn.Args, cmd.Args...)

	spawn.Env = cmd.Env

	spawnR, spawnW, err := os.Pipe()
	if err != nil {
		ready <- err
		return
	}

	spawn.Stdout = spawnW

	spawnOut := bufio.NewReader(spawnR)

	err = p.runner.Background(spawn)
	if err != nil {
		ready <- err
		return
	}

	go spawn.Wait()

	go func() {
		defer func() {
			spawnW.Close()
			spawnR.Close()
		}()

		_, err := spawnOut.ReadBytes('\n')
		if err != nil {
			ready <- err
			return
		}

		ready <- nil

		_, err = spawnOut.ReadBytes('\n')
		if err != nil {
			active <- err
			return
		}

		active <- nil
	}()

	return
}

func (p *Process) Link() {
	p.waitingLinks.Lock()
	defer p.waitingLinks.Unlock()

	p.runningLink.Do(p.runLinker)
}

func (p *Process) Unlink() error {
	<-p.linked

	select {
	case <-p.unlinked:
	default:
		// link already exited
		return nil
	}

	return p.runner.Signal(p.link, os.Interrupt)
}

func (p *Process) Stream() chan warden.ProcessStream {
	return p.registerStream()
}

func (p *Process) runLinker() {
	linkPath := path.Join(p.containerPath, "bin", "iomux-link")
	processDir := path.Join(p.containerPath, "processes", fmt.Sprintf("%d", p.ID))

	p.link = &exec.Cmd{
		Path:   linkPath,
		Args:   []string{"-w", path.Join(processDir, "cursors"), processDir},
		Stdout: p.stdout,
		Stderr: p.stderr,
	}

	p.runner.Start(p.link)

	close(p.linked)

	p.runner.Wait(p.link)

	// if the process is explicitly .Unlinked, block forever; the fact that
	// iomux-link exited should not bubble up to the caller as the linked
	// process didn't actually exit.
	//
	// this is done by .Unlink reading the single value off of .unlinked before
	// interrupting iomux-link, so this read should either block forever in this
	// case or read the value off if the process exited naturally.
	//
	// if .Unlink is called and the value is pulled off, it then knows to not
	// try to terminate the iomux-link, as this only happens if it already
	// exited
	<-p.unlinked

	exitStatus := uint32(255)

	if p.link.ProcessState != nil {
		exitStatus = uint32(p.link.ProcessState.Sys().(syscall.WaitStatus).ExitStatus())
	}

	p.exitStatus = exitStatus

	p.closeStreams()
}

func (p *Process) registerStream() chan warden.ProcessStream {
	p.streamsLock.Lock()
	defer p.streamsLock.Unlock()

	stream := make(chan warden.ProcessStream, 1000)

	p.streams = append(p.streams, stream)

	if p.completed {
		defer p.closeStreams()
	}

	return stream
}

func (p *Process) sendToStreams(chunk warden.ProcessStream) {
	p.streamsLock.RLock()
	defer p.streamsLock.RUnlock()

	for _, stream := range p.streams {
		select {
		case stream <- chunk:
		default:
		}
	}
}

func (p *Process) closeStreams() {
	p.streamsLock.RLock()
	defer p.streamsLock.RUnlock()

	for _, stream := range p.streams {
		stream <- warden.ProcessStream{ExitStatus: &(p.exitStatus)}
		close(stream)
	}

	p.streams = nil
	p.completed = true
}
