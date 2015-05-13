package system

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/pivotal-golang/lager"
)

type ProcessReaper struct {
	mu      *sync.Mutex
	waiting map[int]chan int
	sigChld chan os.Signal
	log     lager.Logger
}

func StartReaper(logger lager.Logger) *ProcessReaper {
	logger.Debug("start-reaper")
	p := &ProcessReaper{
		mu:      new(sync.Mutex),
		waiting: make(map[int]chan int),
		sigChld: make(chan os.Signal, 1000),
		log:     logger,
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

	syscall.Setrlimit(syscall.RLIMIT_NOFILE, &syscall.Rlimit{100000, 100000})

	// TODO: lock current thread to goroutine & unlock after Start

	if err := cmd.Start(); err != nil {
		p.log.Error("failed to start", err, lager.Data{"cmd": cmd})
		return err
	}

	p.log.Info("started", lager.Data{"pid": cmd.Process.Pid, "cmd": cmd})

	p.waiting[cmd.Process.Pid] = make(chan int, 1)
	return nil
}

func (p *ProcessReaper) Wait(cmd *exec.Cmd) byte {
	ch, ok := p.waitChan(cmd.Process.Pid)
	if !ok {
		panic("waited on a process that was never started")
	}

	found := ch != nil
	p.log.Info("wait", lager.Data{"pid": cmd.Process.Pid, "found": found})
	return byte(<-ch)
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
		wpid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, &rusage)

		if wpid == 0 || (wpid == -1 && err.Error() == "no child processes") {
			break
		}

		if err != nil {
			p.log.Error("reaper-wait-error", err, lager.Data{"wpid": wpid})
			break
		}

		p.log.Info("reaped", lager.Data{"pid": wpid, "status": status, "rusage": rusage})

		if ch, ok := p.waitChan(wpid); ok {
			ch <- status.ExitStatus()
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
