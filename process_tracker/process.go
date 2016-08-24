package process_tracker

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/iodaemon/link"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker/writer"
)

//go:generate counterfeiter -o fake_signaller/fake_signaller.go . Signaller
type Signaller interface {
	Signal(*SignalRequest) error
}

//go:generate counterfeiter -o fake_msg_sender/fake_msg_sender.go . MsgSender
type MsgSender interface {
	SendMsg(msg []byte) error
}

type SignalRequest struct {
	Pid    string
	Signal syscall.Signal
	Link   MsgSender
}

type Process struct {
	logger lager.Logger

	id string

	containerPath string
	runner        command_runner.CommandRunner

	runningLink *sync.Once
	linked      chan struct{}
	link        *link.Link

	exited     chan struct{}
	exitStatus int
	exitErr    error

	stdin  writer.FanIn
	stdout writer.FanOut
	stderr writer.FanOut

	signaller Signaller
}

func NewProcess(
	logger lager.Logger,
	id string,
	containerPath string,
	runner command_runner.CommandRunner,
	signaller Signaller,
) *Process {
	return &Process{
		logger: logger,

		id: id,

		containerPath: containerPath,
		runner:        runner,

		runningLink: &sync.Once{},

		linked: make(chan struct{}),

		exited: make(chan struct{}),

		stdin:     writer.NewFanIn(),
		stdout:    writer.NewFanOut(),
		stderr:    writer.NewFanOut(),
		signaller: signaller,
	}
}

func (p *Process) ID() string {
	return p.id
}

func (p *Process) Wait() (int, error) {
	<-p.exited
	return p.exitStatus, p.exitErr
}

func (p *Process) SetTTY(tty garden.TTYSpec) error {
	<-p.linked

	if tty.WindowSize != nil {
		return p.link.SetWindowSize(tty.WindowSize.Columns, tty.WindowSize.Rows)
	}

	return nil
}

func (p *Process) Signal(signal garden.Signal) error {
	<-p.linked

	request := &SignalRequest{Pid: p.id, Link: p.link}

	switch signal {
	case garden.SignalKill:
		request.Signal = syscall.SIGKILL
	case garden.SignalTerminate:
		request.Signal = syscall.SIGTERM
	default:
		return fmt.Errorf("process_tracker: failed to send signal: unknown signal: %d", signal)
	}

	return p.signaller.Signal(request)
}

func (p *Process) Spawn(cmd *exec.Cmd, tty *garden.TTYSpec) (ready, active chan error) {
	ready = make(chan error, 1)
	active = make(chan error, 1)

	spawnPath := path.Join(p.containerPath, "bin", "iodaemon")
	processSock := path.Join(p.containerPath, "processes", fmt.Sprintf("%s.sock", p.ID()))
	straceOutput := path.Join(p.containerPath, "processes", fmt.Sprintf("%s.strace", p.ID()))

	os.MkdirAll(filepath.Dir(processSock), 0755)

	bashFlags := []string{
		"-c",
		// spawn but not as a child process (fork off in the bash subprocess).
		`strace -f -tt -T -o ` + straceOutput + " " + spawnPath + ` "$@" &`,
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

	spawnErrR, err := spawn.StderrPipe()
	if err != nil {
		ready <- err
		return
	}
	spawnErr := bufio.NewReader(spawnErrR)

	err = p.runner.Start(spawn)
	if err != nil {
		ready <- err
		return
	}

	go func() {
		waitFor := func(expectedLog string) error {
			p.logger.Info("waiting for " + expectedLog)
			log, err := spawnOut.ReadBytes('\n')
			if err != nil {
				stderrContents, readErr := ioutil.ReadAll(spawnErr)
				if readErr != nil {
					p.logger.Error("errored waiting for "+expectedLog, err, lager.Data{"log": string(log), "readErr": readErr})
					return err
				}

				p.logger.Error("errored waiting for "+expectedLog, err, lager.Data{"log": string(log), "stderr": string(stderrContents)})
				return err
			}

			if !strings.HasPrefix(string(log), expectedLog) {
				stderrContents, readErr := ioutil.ReadAll(spawnErr)
				if readErr != nil {
					p.logger.Error("errored waiting for "+expectedLog, err, lager.Data{"log": string(log), "readErr": readErr})
					return err
				}

				p.logger.Error("errored waiting for "+expectedLog, err, lager.Data{"stderr": string(stderrContents), "log": string(log)})
				return errors.New("mismatched log from iodaemon")
			}

			return nil
		}

		if waitFor("ready") != nil {
			return
		}

		ready <- nil

		if waitFor("listener-accepted") != nil {
			return
		}
		if waitFor("unix-rights") != nil {
			return
		}
		if waitFor("write-msg-unix") != nil {
			return
		}
		if waitFor("accepted-connection") != nil {
			return
		}
		if waitFor("cmd-start") != nil {
			return
		}
		if waitFor("cmd-started") != nil {
			return
		}

		if waitFor("active") != nil {
			return
		}

		active <- nil

		spawn.Wait()

		os.Remove(straceOutput)
	}()

	return
}

func (p *Process) Link() {
	p.runningLink.Do(p.runLinker)
}

func (p *Process) Attach(processIO garden.ProcessIO) {
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

// This is guarded by runningLink so will only run once per Process per garden.
func (p *Process) runLinker() {
	processSock := path.Join(p.containerPath, "processes", fmt.Sprintf("%s.sock", p.ID()))

	p.logger.Info("creating-link-to-iodaemon", lager.Data{"socket-path": processSock})
	link, err := link.Create(p.logger.Session("link-create"), processSock, p.stdout, p.stderr)
	if err != nil {
		p.logger.Error("creating-link-failed", err, lager.Data{"socket-path": processSock})
		p.completed(-1, err)
		return
	}
	p.logger.Info("created-link-to-iodaemon", lager.Data{"socket-path": processSock, "err": err})

	p.stdin.AddSink(link)

	p.link = link
	close(p.linked)

	p.completed(p.link.Wait())

	// don't leak stdin pipe
	p.stdin.Close()
}

func (p *Process) completed(exitStatus int, err error) {
	p.exitStatus = exitStatus
	p.exitErr = err
	close(p.exited)
}
