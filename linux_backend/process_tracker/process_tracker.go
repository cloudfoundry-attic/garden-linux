package process_tracker

import (
	"fmt"
	"os/exec"
	"sync"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry/gunk/command_runner"
)

type ProcessTracker interface {
	Run(*exec.Cmd, warden.ProcessIO, *warden.TTYSpec) (warden.Process, error)
	Attach(uint32, warden.ProcessIO) (warden.Process, error)
	Restore(processID uint32)
	ActiveProcesses() []warden.Process
}

type processTracker struct {
	containerPath string
	runner        command_runner.CommandRunner

	processes      map[uint32]*Process
	nextProcessID  uint32
	processesMutex *sync.RWMutex
}

type UnknownProcessError struct {
	ProcessID uint32
}

func (e UnknownProcessError) Error() string {
	return fmt.Sprintf("unknown process: %d", e.ProcessID)
}

func New(containerPath string, runner command_runner.CommandRunner) ProcessTracker {
	return &processTracker{
		containerPath: containerPath,
		runner:        runner,

		processes:      make(map[uint32]*Process),
		processesMutex: new(sync.RWMutex),

		nextProcessID: 1,
	}
}

func (t *processTracker) Run(cmd *exec.Cmd, processIO warden.ProcessIO, tty *warden.TTYSpec) (warden.Process, error) {
	t.processesMutex.Lock()

	processID := t.nextProcessID
	t.nextProcessID++

	process := NewProcess(processID, t.containerPath, t.runner)

	t.processes[processID] = process

	t.processesMutex.Unlock()

	ready, active := process.Spawn(cmd, tty)

	err := <-ready
	if err != nil {
		return nil, err
	}

	process.Attach(processIO)

	go t.link(processID)

	err = <-active
	if err != nil {
		return nil, err
	}

	return process, nil
}

func (t *processTracker) Attach(processID uint32, processIO warden.ProcessIO) (warden.Process, error) {
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

func (t *processTracker) Restore(processID uint32) {
	t.processesMutex.Lock()

	process := NewProcess(processID, t.containerPath, t.runner)

	t.processes[processID] = process

	if processID >= t.nextProcessID {
		t.nextProcessID = processID + 1
	}

	go t.link(processID)

	t.processesMutex.Unlock()
}

func (t *processTracker) ActiveProcesses() []warden.Process {
	t.processesMutex.RLock()
	defer t.processesMutex.RUnlock()

	processes := make([]warden.Process, len(t.processes))

	i := 0
	for _, process := range t.processes {
		processes[i] = process
		i++
	}

	return processes
}

func (t *processTracker) link(processID uint32) {
	t.processesMutex.RLock()
	process, ok := t.processes[processID]
	t.processesMutex.RUnlock()

	if !ok {
		return
	}

	defer t.unregister(processID)

	process.Link()

	return
}

func (t *processTracker) unregister(processID uint32) {
	t.processesMutex.Lock()
	defer t.processesMutex.Unlock()

	delete(t.processes, processID)
}
