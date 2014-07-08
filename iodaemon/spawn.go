package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kr/pty"
)

func spawn(socketPath string, path string, argv []string, timeout time.Duration, withTty bool) {
	err := os.MkdirAll(filepath.Dir(socketPath), 0755)
	if err != nil {
		fatal(err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		fatal(err)
	}

	var stdinW, stdoutR, stderrR *os.File

	bin, err := exec.LookPath(path)
	if err != nil {
		fatal(err)
	}

	cmd := &exec.Cmd{
		Path: bin,
		Args: argv,
	}

	if withTty {
		pty, tty, err := pty.Open()
		if err != nil {
			fatal(err)
		}

		stdinW = pty
		stdoutR = pty
		stderrR = pty

		cmd.Stdin = tty
		cmd.Stdout = tty
		cmd.Stderr = tty
		cmd.SysProcAttr = &syscall.SysProcAttr{Setctty: true, Setsid: true}
	} else {
		cmd.Stdin, stdinW, err = os.Pipe()
		if err != nil {
			fatal(err)
		}

		stdoutR, cmd.Stdout, err = os.Pipe()
		if err != nil {
			fatal(err)
		}

		stderrR, cmd.Stderr, err = os.Pipe()
		if err != nil {
			fatal(err)
		}
	}

	statusR, statusW, err := os.Pipe()
	if err != nil {
		fatal(err)
	}

	fmt.Println("ready")

	started := false

	for {
		conn, err := listener.Accept()
		if err != nil {
			fatal(err)
			break
		}

		rights := syscall.UnixRights(
			int(stdinW.Fd()),
			int(stdoutR.Fd()),
			int(stderrR.Fd()),
			int(statusR.Fd()),
		)

		_, _, err = conn.(*net.UnixConn).WriteMsgUnix([]byte{}, rights, nil)
		if err != nil {
			log.Println("ERROR WRITING UNIX:", err)
			break
		}

		sync := make([]byte, 1)
		n, err := conn.Read(sync)
		if n != 1 || err != nil {
			log.Println("ERROR SYNCING:", err)
			break
		}

		if !started {
			err = cmd.Start()
			if err != nil {
				fatal(err)
			}

			go func() {
				cmd.Wait()

				if cmd.ProcessState != nil {
					fmt.Fprintf(statusW, "%d\n", cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus())
				} else {
					fmt.Fprintf(statusW, "255\n")
				}

				os.Exit(0)
			}()

			fmt.Println("pid:", cmd.Process.Pid)

			// detach from parent
			os.Stdin.Close()
			os.Stdout.Close()
			os.Stderr.Close()

			started = true
		}
	}
}

func fatal(err error) {
	println("fatal: " + err.Error())
	os.Exit(1)
}
