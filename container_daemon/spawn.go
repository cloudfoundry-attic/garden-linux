package container_daemon

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
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

	//		pid := cmd.Process.Pid
	//	go func( /*pid int, file *os.File*/ ) {
	//		logFile, err := os.OpenFile(fmt.Sprintf("/tmp/initd-%d-pipe.txt", pid), os.O_CREATE|os.O_SYNC|os.O_WRONLY, 0755)
	//		if err != nil {
	//			return
	//		}

	//		for /*file.Fd() >= 0*/ {
	//			fmt.Fprintln(logFile, "spawn.go read edn of pipe still open", time.Now().Format(time.RFC3339), cmd)
	//			time.Sleep(time.Millisecond * 100)
	//		}
	//		fmt.Fprintln(logFile, "spawn.go read end of pipe is closed", time.Now().Format(time.RFC3339), cmd)
	//	}( /*pid, pipes[0].r*/ )

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

		pid := cmd.Process.Pid
		logFile, err := os.OpenFile(fmt.Sprintf("/tmp/initd-%d-stdout.txt", pid), os.O_CREATE|os.O_SYNC|os.O_WRONLY, 0755)
		if err != nil {
			return
		}

		fmt.Fprintln(logFile, "spawn.go about to issue Wait", fmt.Sprintf("%.9f", float64(time.Now().UnixNano())/1e9), cmd)
		status := runner.Wait(cmd)
		fmt.Fprintln(logFile, "spawn.go returned from Wait", fmt.Sprintf("%.9f", float64(time.Now().UnixNano())/1e9), cmd)
		exitW.Write([]byte{status})
	}()

	return exitR, nil
}
