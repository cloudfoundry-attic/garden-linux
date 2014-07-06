package process_tracker

import (
	"bufio"
	"errors"
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
	id uint32

	containerPath string
	runner        command_runner.CommandRunner

	waitingLinks *sync.Mutex
	runningLink  *sync.Once
	link         *exec.Cmd

	linked   chan struct{}
	unlinked <-chan struct{}

	exitStatus int
	exitErr    error
	done       bool
	doneL      *sync.Cond

	stdout *fanoutWriter
	stderr *fanoutWriter
}

func NewProcess(
	id uint32,
	containerPath string,
	runner command_runner.CommandRunner,
) *Process {
	unlinked := make(chan struct{}, 1)
	unlinked <- struct{}{}

	p := &Process{
		id: id,

		containerPath: containerPath,
		runner:        runner,

		waitingLinks: &sync.Mutex{},
		runningLink:  &sync.Once{},
		linked:       make(chan struct{}),
		unlinked:     unlinked,

		doneL: sync.NewCond(&sync.Mutex{}),
	}

	p.stdout = &fanoutWriter{}
	p.stderr = &fanoutWriter{}

	return p
}

func (p *Process) ID() uint32 {
	return p.id
}

func (p *Process) Wait() (int, error) {
	p.doneL.L.Lock()

	for !p.done {
		p.doneL.Wait()
	}

	defer p.doneL.L.Unlock()

	return p.exitStatus, p.exitErr
}

func (p *Process) Spawn(cmd *exec.Cmd) (ready, active chan error) {
	ready = make(chan error, 1)
	active = make(chan error, 1)

	spawnPath := path.Join(p.containerPath, "bin", "iomux-spawn")
	processDir := path.Join(p.containerPath, "processes", fmt.Sprintf("%d", p.ID()))

	err := os.MkdirAll(processDir, 0755)
	if err != nil {
		ready <- err
		return
	}

	spawn := &exec.Cmd{
		Path:  "bash",
		Stdin: cmd.Stdin,
	}

	// spawn but not as a child process (fork off in the bash subprocess). pipe
	// 'cat' to it to keep stdin connected.
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

func (p *Process) Attach(processIO warden.ProcessIO) {
	if processIO.Stdout != nil {
		p.stdout.AddSink(processIO.Stdout)
	}

	if processIO.Stderr != nil {
		p.stderr.AddSink(processIO.Stderr)
	}
}

func (p *Process) runLinker() {
	linkPath := path.Join(p.containerPath, "bin", "iomux-link")
	processDir := path.Join(p.containerPath, "processes", fmt.Sprintf("%d", p.ID()))

	p.link = &exec.Cmd{
		Path:   linkPath,
		Args:   []string{"-w", path.Join(processDir, "cursors"), processDir},
		Stdout: p.stdout,
		Stderr: p.stderr,
	}

	defer func() {
		err := p.stdout.Close()
		if err != nil {
			p.completed(-1, err)
		}

		err = p.stderr.Close()
		if err != nil {
			p.completed(-1, err)
		}
	}()

	err := p.runner.Start(p.link)
	if err != nil {
		p.completed(-1, err)
		return
	}

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

	if p.link.ProcessState != nil {
		p.completed(p.link.ProcessState.Sys().(syscall.WaitStatus).ExitStatus(), nil)
	} else {
		// this really should not happen, since we called .Wait()
		p.completed(-1, errors.New("could not determine exit status"))
	}
}

func (p *Process) completed(exitStatus int, err error) {
	p.doneL.L.Lock()

	if p.done {
		p.doneL.L.Unlock()
		return
	}

	p.done = true
	p.exitErr = err
	p.exitStatus = exitStatus
	p.doneL.L.Unlock()

	p.doneL.Broadcast()
}
