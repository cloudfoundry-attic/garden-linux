package system

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"code.cloudfoundry.org/lager"
)

type ProcessReaper struct {
	mu            *sync.Mutex
	waiting       map[int]chan int
	monitoredPids map[int]bool // pids which we launched, to avoid confusion with processes launched by children inside the container
	sigChld       chan os.Signal
	log           lager.Logger

	wait4 Wait4Func
}

type Wait4Func func(pid int, wstatus *syscall.WaitStatus, options int, rusage *syscall.Rusage) (wpid int, err error)

func StartReaper(logger lager.Logger, waitSyscall Wait4Func) *ProcessReaper {
	logger.Debug("start-reaper")
	p := &ProcessReaper{
		mu:            new(sync.Mutex),
		waiting:       make(map[int]chan int),
		monitoredPids: make(map[int]bool),
		sigChld:       make(chan os.Signal, 1000),
		log:           logger,

		wait4: waitSyscall,
	}

	signal.Notify(p.sigChld, syscall.SIGCHLD)
	go p.reapAll()
	return p
}

func (p *ProcessReaper) Stop() {
	signal.Stop(p.sigChld)
}

func (p *ProcessReaper) Start(cmd *exec.Cmd) error {
	// Lock before starting the command to ensure p.waiting is set before Wait attempts to read it.
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := cmd.Start(); err != nil {
		p.log.Error("failed to start", err, lager.Data{"cmd": cmd})
		return err
	}

	p.log.Info("started", lager.Data{"pid": cmd.Process.Pid, "cmd": cmd})

	p.waiting[cmd.Process.Pid] = make(chan int, 1)
	p.monitoredPids[cmd.Process.Pid] = true
	return nil
}

func (p *ProcessReaper) Wait(cmd *exec.Cmd) byte {
	ch, ok := p.waitChan(cmd.Process.Pid)
	if !ok {
		panic("waited on a process that was never started")
	}

	found := ch != nil
	p.log.Info("reaper-receiving-process-exit-status", lager.Data{"pid": cmd.Process.Pid, "found": found})
	exitStatus := byte(<-ch)
	p.log.Debug("reaper-wait-received-process-exit-status", lager.Data{"pid": cmd.Process.Pid, "exitStatus": exitStatus})
	return exitStatus
}

func (p *ProcessReaper) reapAll() {
	for {
		p.log.Debug("reaper-waiting-for-SIGCHLD")
		<-p.sigChld
		p.reap()
	}
}

func (p *ProcessReaper) reap() {
	for {
		p.log.Debug("reap")
		var status syscall.WaitStatus
		var rusage syscall.Rusage
		wpid, err := p.wait4(-1, &status, syscall.WNOHANG, &rusage)

		if wpid == 0 || (wpid == -1 && err.Error() == "no child processes") {
			break
		}

		if err != nil {
			p.log.Error("reaper-wait-error", err, lager.Data{"wpid": wpid})
			break
		}

		p.log.Info("reaped", lager.Data{"pid": wpid, "status": status, "rusage": rusage})

		p.mu.Lock()
		waitPid := p.monitoredPids[wpid]
		p.mu.Unlock()

		ch, isWaiting := p.waitChan(wpid)
		if waitPid && isWaiting {
			ch <- status.ExitStatus()
			p.unmonitorPid(wpid)

			p.log.Info("wait-once-sent-exit-status", lager.Data{"pid": wpid, "status": status, "rusage": rusage})
		} else {
			p.log.Info("wait-once-not-found", lager.Data{"pid": wpid, "status": status, "rusage": rusage})
		}
	}
}

func (p *ProcessReaper) waitChan(pid int) (chan int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	wChan, ok := p.waiting[pid]
	return wChan, ok
}

func (p *ProcessReaper) unmonitorPid(pid int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.monitoredPids, pid)
}
