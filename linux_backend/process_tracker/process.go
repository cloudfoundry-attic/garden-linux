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
	"github.com/kr/pty"

	"github.com/cloudfoundry-incubator/warden-linux/ptyutil"
)

type Process struct {
	id      uint32
	withTty bool

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

	pty *os.File

	stdin  *faninWriter
	stdout *fanoutWriter
	stderr *fanoutWriter
}

func NewProcess(
	id uint32,
	withTty bool,
	containerPath string,
	runner command_runner.CommandRunner,
) *Process {
	unlinked := make(chan struct{}, 1)
	unlinked <- struct{}{}

	return &Process{
		id:      id,
		withTty: withTty,

		containerPath: containerPath,
		runner:        runner,

		waitingLinks: &sync.Mutex{},
		runningLink:  &sync.Once{},
		linked:       make(chan struct{}),
		unlinked:     unlinked,

		doneL: sync.NewCond(&sync.Mutex{}),

		stdin:  &faninWriter{hasSink: make(chan struct{})},
		stdout: &fanoutWriter{},
		stderr: &fanoutWriter{},
	}
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

func (p *Process) SetTTY(tty warden.TTYSpec) error {
	<-p.linked

	if p.pty == nil {
		return nil
	}

	if tty.WindowSize != nil {
		err := ptyutil.SetWinSize(p.pty, tty.WindowSize.Columns, tty.WindowSize.Rows)
		if err != nil {
			return err
		}
	}

	return p.link.Process.Signal(syscall.SIGWINCH)
}

func (p *Process) WithTTY() bool {
	return p.withTty
}

func (p *Process) Spawn(cmd *exec.Cmd, tty *warden.TTYSpec) (ready, active chan error) {
	ready = make(chan error, 1)
	active = make(chan error, 1)

	spawnPath := path.Join(p.containerPath, "bin", "iodaemon")
	processSock := path.Join(p.containerPath, "processes", fmt.Sprintf("%d.sock", p.ID()))

	bashFlags := []string{
		"-c",
		// spawn but not as a child process (fork off in the bash subprocess).
		spawnPath + ` "$@" &`,
		spawnPath,
	}

	if tty != nil {
		bashFlags = append(bashFlags, "-tty")

		if tty.WindowSize != nil {
			bashFlags = append(
				bashFlags,
				fmt.Sprintf("-windowColumns=%d", tty.WindowSize.Columns),
				fmt.Sprintf("-windowRows=%d", tty.WindowSize.Rows),
			)
		}
	}

	bashFlags = append(bashFlags, "spawn", processSock)

	spawn := exec.Command("bash", append(bashFlags, cmd.Args...)...)
	spawn.Env = cmd.Env

	spawnR, err := spawn.StdoutPipe()
	if err != nil {
		ready <- err
		return
	}

	spawnOut := bufio.NewReader(spawnR)

	err = p.runner.Background(spawn)
	if err != nil {
		ready <- err
		return
	}

	go func() {
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

		spawn.Wait()
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
	if processIO.Stdin != nil {
		p.stdin.AddSource(processIO.Stdin)
	}

	if processIO.Stdout != nil {
		p.stdout.AddSink(processIO.Stdout)
	}

	if processIO.Stderr != nil {
		p.stderr.AddSink(processIO.Stderr)
	}
}

func (p *Process) runLinker() {
	linkPath := path.Join(p.containerPath, "bin", "iodaemon")
	processSock := path.Join(p.containerPath, "processes", fmt.Sprintf("%d.sock", p.ID()))

	var inR, inW *os.File
	var err error

	if p.withTty {
		pty, tty, err := pty.Open()
		if err != nil {
			p.completed(-1, err)
			return
		}

		err = ptyutil.SetRaw(tty)
		if err != nil {
			p.completed(-1, err)
			return
		}

		inR = tty
		inW = pty

		p.pty = pty
	} else {
		inR, inW, err = os.Pipe()
		if err != nil {
			p.completed(-1, err)
			return
		}
	}

	p.stdin.AddSink(inW)

	p.link = exec.Command(
		linkPath,
		fmt.Sprintf("-tty=%v", p.withTty),
		"link",
		processSock,
	)

	p.link.Stdin = inR
	p.link.Stdout = p.stdout
	p.link.Stderr = p.stderr

	err = p.runner.Start(p.link)
	if err != nil {
		p.completed(-1, err)
		return
	}

	// close our copy of the process's end of the pipe now that it's spawned
	inR.Close()

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

	// close our end of the pipe so it doesn't leak
	p.stdin.Close()

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
