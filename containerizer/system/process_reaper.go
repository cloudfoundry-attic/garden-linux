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
	p := &ProcessReaper{
		mu:      new(sync.Mutex),
		waiting: make(map[int]chan int),
		sigChld: make(chan os.Signal, 10),
		log:     logger,
	}

	signal.Notify(p.sigChld, syscall.SIGCHLD)
	go p.WaitAll()
	return p
}

func (p *ProcessReaper) Stop() {
	signal.Stop(p.sigChld)
}

func (p *ProcessReaper) WaitAll() {
	for {
		select {
		case <-p.sigChld:
			p.waitOnce()
		}
	}
}

func (p *ProcessReaper) waitOnce() {
	var status syscall.WaitStatus
	var rusage syscall.Rusage
	wpid, err := syscall.Wait4(-1, &status, 0, &rusage)

	if err != nil {
		p.log.Error("system: process reaper wait: %s", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if ch, ok := p.waiting[wpid]; ok {
		ch <- status.ExitStatus()
	}
}

func (p *ProcessReaper) Start(cmd *exec.Cmd) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return err
	}

	p.waiting[cmd.Process.Pid] = make(chan int)
	return nil
}

func (p *ProcessReaper) Wait(cmd *exec.Cmd) (byte, error) {
	p.mu.Lock()
	ch := p.waiting[cmd.Process.Pid]
	p.mu.Unlock()
	return byte(<-ch), nil
}
