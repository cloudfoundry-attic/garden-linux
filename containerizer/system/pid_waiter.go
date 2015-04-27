package system

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
)

type PidWaiter struct {
	mu      *sync.Mutex
	waiting map[int]chan int
}

func StartReaper() *PidWaiter {
	p := &PidWaiter{
		mu:      new(sync.Mutex),
		waiting: make(map[int]chan int),
	}

	go p.WaitAll()
	return p
}

func (p *PidWaiter) WaitAll() {
	var status syscall.WaitStatus
	var rusage syscall.Rusage
	fmt.Printf("wait\n")
	wpid, _ := syscall.Wait4(-1, &status, 0, &rusage)
	fmt.Printf("got exit for %d\n", wpid)

	p.mu.Lock()
	defer p.mu.Unlock()
	if ch, ok := p.waiting[wpid]; ok {
		fmt.Printf("send baack exit for %d\n", wpid)
		ch <- status.ExitStatus()
	}
}

func (p *PidWaiter) Start(cmd *exec.Cmd) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	err := cmd.Start()
	p.waiting[cmd.Process.Pid] = make(chan int)

	fmt.Printf("started %s \n", cmd.Process.Pid)
	return err
}

func (p *PidWaiter) Wait(cmd *exec.Cmd) (byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch := p.waiting[cmd.Process.Pid]
	fmt.Printf("wait %s, %s \n", cmd.Process.Pid, ch)
	return byte(<-ch), nil
}
