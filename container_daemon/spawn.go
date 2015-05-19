package container_daemon

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

type Spawn struct {
	PTY    PTYOpener
	Runner Runner
}

//go:generate counterfeiter -o fake_runner/fake_runner.go . Runner
type Runner interface {
	Start(cmd *exec.Cmd) error
	Wait(cmd *exec.Cmd) byte
}

//go:generate counterfeiter -o fake_ptyopener/fake_ptyopener.go . PTYOpener
type PTYOpener interface {
	Open() (pty *os.File, tty *os.File, err error)
}

func (w *Spawn) Spawn(cmd *exec.Cmd, tty bool) ([]*os.File, error) {
	if tty {
		return w.spawnWithTty(cmd)
	} else {
		return w.spawnNoninteractive(cmd)
	}
}

func (w *Spawn) spawnWithTty(cmd *exec.Cmd) ([]*os.File, error) {
	pty, tty, err := w.PTY.Open()
	if err != nil {
		return nil, fmt.Errorf("container_daemon: open pipe: %s", err)
	}

	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Setctty = true
	cmd.SysProcAttr.Setsid = true

	exitFd, err := wireExit(cmd, w.Runner)
	if err != nil {
		pty.Close()
		tty.Close()
		return nil, err
	}

	return []*os.File{pty, exitFd}, err
}

func (w *Spawn) spawnNoninteractive(cmd *exec.Cmd) ([]*os.File, error) {
	var pipes [3]struct {
		r *os.File
		w *os.File
	}

	var err error
	for i := 0; i < 3; i++ {
		pipes[i].r, pipes[i].w, err = os.Pipe()
		if err != nil {
			return nil, fmt.Errorf("container_daemon: create pipe: %s", err)
		}
	}

	cmd.Stdin = pipes[0].r
	cmd.Stdout = pipes[1].w
	cmd.Stderr = pipes[2].w

	exitStatusR, err := wireExit(cmd, w.Runner)
	if err != nil {
		for _, p := range pipes {
			p.r.Close()
			p.w.Close()
		}

		return nil, err
	}

	return []*os.File{pipes[0].w, pipes[1].r, pipes[2].r, exitStatusR}, nil
}

func wireExit(cmd *exec.Cmd, runner Runner) (*os.File, error) {
	exitR, exitW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("container_daemon: create pipe: %s", err)
	}

	if err := runner.Start(cmd); err != nil {
		return nil, fmt.Errorf("container_daemon: start: %s", err)
	}

	go func() {
		defer exitW.Close()
		status := runner.Wait(cmd)
		exitW.Write([]byte{status})
	}()

	return exitR, nil
}
