package main

import (
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	debugPkg "runtime/debug"
	"strconv"
	"syscall"
	"time"

	"io"

	linkpkg "github.com/cloudfoundry-incubator/garden-linux/old/iodaemon/link"
	"github.com/kr/pty"
)

func spawn(
	socketPath string,
	argv []string,
	timeout time.Duration,
	withTty bool,
	windowColumns int,
	windowRows int,
	debug bool,
	terminate func(int),
	inStream io.ReadCloser,
	outStream io.WriteCloser,
	errStream io.WriteCloser,
) {
	fatal := func(err error) {
		debugPkg.PrintStack()
		fmt.Fprintln(errStream, "fatal: "+err.Error())
		terminate(1)
	}

	err := os.MkdirAll(filepath.Dir(socketPath), 0755)
	if err != nil {
		fatal(err)
	}

	if debug {
		ownPid := os.Getpid()

		traceOut, err := os.Create(socketPath + ".trace")
		if err != nil {
			fatal(err)
		}

		strace := exec.Command("strace", "-f", "-s", "10240", "-p", strconv.Itoa(ownPid))
		strace.Stdout = traceOut
		strace.Stderr = traceOut

		err = strace.Start()
		if err != nil {
			fatal(err)
		}
	}

    // Delete socketPath if it exists to avoid bind failures.
    err = os.Remove(socketPath)
    if err != nil && !os.IsNotExist(err) {
        fatal(err)
    }

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		fatal(err)
        return
	}

	bin, err := exec.LookPath(argv[0])
	if err != nil {
		fatal(err)
	}

	cmd := child(bin, argv)

	var stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW *os.File
	if withTty {
		stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW, err = glueTty(windowColumns, windowRows)
		cmd.SysProcAttr.Setctty = true
		cmd.SysProcAttr.Setsid = true
	} else {
		stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW, err = glueNoTty(windowColumns, windowRows)
	}

	if err != nil {
		fatal(err)
	}

	cmd.Stdin = stdinR
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	statusR, statusW, err := os.Pipe()
	if err != nil {
		fatal(err)
	}

	fmt.Fprintln(outStream, "ready")

	started := false

	for {
		conn, err := listener.Accept()
		if err != nil {
			fatal(err)
			break
		}

		rights := syscall.UnixRights(
			int(stdoutR.Fd()),
			int(stderrR.Fd()),
			int(statusR.Fd()),
		)

		_, _, err = conn.(*net.UnixConn).WriteMsgUnix([]byte{}, rights, nil)
		if err != nil {
			fatal(err)
			break
		}

		if !started {
			err := cmd.Start()
			if err != nil {
				fatal(err)
			}

			// close no longer relevant pipe ends
			// this closes tty 3 times but that's OK
			stdinR.Close()
			stdoutW.Close()
			stderrW.Close()

			fmt.Fprintln(outStream, "pid:", cmd.Process.Pid)

			go func() {
				cmd.Wait()

				if cmd.ProcessState != nil {
					fmt.Fprintf(statusW, "%d\n", cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus())
				}

				terminate(0)
			}()

			// detach from parent process
			inStream.Close()
			outStream.Close()
			errStream.Close()

			started = true
		}

		decoder := gob.NewDecoder(conn)

		for {
			var input linkpkg.Input
			err := decoder.Decode(&input)
			if err != nil {
				break
			}

			if input.WindowSize != nil {
				setWinSize(stdinW, input.WindowSize.Columns, input.WindowSize.Rows)
				cmd.Process.Signal(syscall.SIGWINCH)
			} else if input.EOF {
				stdinW.Sync()
				err := stdinW.Close()
				if withTty {
					cmd.Process.Signal(syscall.SIGHUP)
				}
				if err != nil {
					conn.Close()
					break
				}
			} else {
				_, err := stdinW.Write(input.Data)
				if err != nil {
					conn.Close()
					break
				}
			}
		}
	}
}

func glueNoTty(windowColumns int, windowRows int) (stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW *os.File, err error) {
	// stderr will not be assigned in the case of a tty, so make
	// a dummy pipe to send across instead
	stderrR, stderrW, err = os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	stdinR, stdinW, err = os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	stdoutR, stdoutW, err = os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	return
}

func glueTty(windowColumns int, windowRows int) (stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW *os.File, err error) {
	// stderr will not be assigned in the case of a tty, so ensure it will return EOF on read
    stderrR, err = os.Open("/dev/null")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	pty, tty, err := pty.Open()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	// do NOT assign stderrR to pty; the receiving end should only receive one
	// pty output stream, as they're both the same fd

	stdinW = pty
	stdoutR = pty

	stdinR = tty
	stdoutW = tty
	stderrW = tty

	setWinSize(stdinW, windowColumns, windowRows)

	return
}
