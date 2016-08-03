package process_tracker

import (
	"fmt"
	"os/exec"
	"sync"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_process_tracker/fake_process_tracker.go . ProcessTracker
type ProcessTracker interface {
	Run(processID string, cmd *exec.Cmd, io garden.ProcessIO, tty *garden.TTYSpec, signaller Signaller) (garden.Process, error)
	Attach(processID string, io garden.ProcessIO) (garden.Process, error)
	Restore(processID string, signaller Signaller)
	ActiveProcesses() []garden.Process
}

type processTracker struct {
	logger        lager.Logger
	containerPath string
	runner        command_runner.CommandRunner

	processes      map[string]*Process
	processesMutex *sync.RWMutex
}

type UnknownProcessError struct {
	ProcessID string
}

func (e UnknownProcessError) Error() string {
	return fmt.Sprintf("process_tracker: unknown process: %s", e.ProcessID)
}

func New(logger lager.Logger, containerPath string, runner command_runner.CommandRunner) ProcessTracker {
	return &processTracker{
		logger: logger,

		containerPath: containerPath,
		runner:        runner,

		processesMutex: new(sync.RWMutex),
		processes:      make(map[string]*Process),
	}
}

func (t *processTracker) Run(processID string, cmd *exec.Cmd, processIO garden.ProcessIO, tty *garden.TTYSpec, signaller Signaller) (garden.Process, error) {
	t.processesMutex.Lock()
	process := NewProcess(t.logger.Session("process", lager.Data{"id": processID}), processID, t.containerPath, t.runner, signaller)
	t.processes[processID] = process
	t.processesMutex.Unlock()

	t.logger.Info("run-spawning", lager.Data{"id": processID, "cmd": cmd})
	ready, active := process.Spawn(cmd, tty)
	t.logger.Info("run-spawned", lager.Data{"id": processID, "cmd": cmd})

	t.logger.Info("waiting-iodaemon-ready")
	err := <-ready
	if err != nil {
		t.logger.Error("reading-ready-failed", err)
		return nil, err
	}
	t.logger.Info("iodaemon-ready")

	t.logger.Info("attaching")
	process.Attach(processIO)
	t.logger.Info("attached")

	go t.link(process.ID())

	t.logger.Info("waiting-iodaemon-active")
	err = <-active
	if err != nil {
		t.logger.Error("reading-active-failed", err)
		return nil, err
	}
	t.logger.Info("iodaemon-active")

	return process, nil
}

func (t *processTracker) Attach(processID string, processIO garden.ProcessIO) (garden.Process, error) {
	t.processesMutex.RLock()
	process, ok := t.processes[processID]
	t.processesMutex.RUnlock()

	if !ok {
		return nil, UnknownProcessError{processID}
	}

	process.Attach(processIO)

	go t.link(processID)

	return process, nil
}

func (t *processTracker) Restore(processID string, signaller Signaller) {
	t.processesMutex.Lock()

	process := NewProcess(t.logger, processID, t.containerPath, t.runner, signaller)

	t.processes[processID] = process

	go t.link(processID)

	t.processesMutex.Unlock()
}

func (t *processTracker) ActiveProcesses() []garden.Process {
	t.processesMutex.RLock()
	defer t.processesMutex.RUnlock()

	processes := make([]garden.Process, 0)
	for _, process := range t.processes {
		processes = append(processes, process)
	}

	return processes
}

func (t *processTracker) link(processID string) {
	t.logger.Info("link-start")
	t.processesMutex.RLock()
	process, ok := t.processes[processID]
	t.processesMutex.RUnlock()
	t.logger.Info("got-process-from-mutex-map")

	if !ok {
		return
	}

	defer t.unregister(processID)

	process.Link()

	return
}

func (t *processTracker) unregister(processID string) {
	t.processesMutex.Lock()
	defer t.processesMutex.Unlock()

	t.logger.Info("unregistering", lager.Data{"id": processID})
	delete(t.processes, processID)
}
