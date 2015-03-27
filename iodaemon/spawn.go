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
	"sync"
	"syscall"
	"time"

	"io"

	linkpkg "github.com/cloudfoundry-incubator/garden-linux/iodaemon/link"
	"github.com/kr/pty"
)

// spawn listens on a unix socket at the given socketPath and when the first connection
// is received, starts a child process.
func spawn(
	socketPath string,
	argv []string,
	timeout time.Duration,
	withTty bool,
	windowColumns int,
	windowRows int,
	debug bool,
	terminate chan int,
	notifyStream io.WriteCloser,
	errStream io.WriteCloser,
) {
	var listener net.Listener

	fatal := func(err error) {
		if debug {
			debugPkg.PrintStack()
		}
		fmt.Fprintln(errStream, "fatal: "+err.Error())
		if listener != nil {
			listener.Close()
		}
		terminate <- 1
	}

	if debug {
		enableTracing(socketPath, fatal)
	}

	listener, err := listen(socketPath)
	if err != nil {
		fatal(err)
		return
	}

	executablePath, err := exec.LookPath(argv[0])
	if err != nil {
		fatal(err)
		return
	}

	cmd := child(executablePath, argv)

	var stdinW, stdoutR, stderrR *os.File
	if withTty {
		cmd.Stdin, stdinW, stdoutR, cmd.Stdout, stderrR, cmd.Stderr, err = createTtyPty(windowColumns, windowRows)
		cmd.SysProcAttr.Setctty = true
		cmd.SysProcAttr.Setsid = true
	} else {
		cmd.Stdin, stdinW, stdoutR, cmd.Stdout, stderrR, cmd.Stderr, err = createPipes()
	}

	if err != nil {
		fatal(err)
		return
	}

	statusR, statusW, err := os.Pipe()
	if err != nil {
		fatal(err)
		return
	}

	acceptConn := func(stopAccepting chan bool) (net.Conn, error) {
		notify(notifyStream, "ready")
		conn, err := acceptConnection(listener, stdoutR, stderrR, statusR)
		if err != nil {
			select {
			case <-stopAccepting:
				return nil, err
			default:
				fatal(err)
				return nil, err
			}
		}
		return conn, nil
	}

	processConn := func(conn net.Conn) {
		processLinkRequests(conn, stdinW, cmd, withTty)
	}

	startChild := func() error {
		err := cmd.Start()
		if err != nil {
			fatal(err)
			return err
		}

		notify(notifyStream, "active")
		notifyStream.Close()
		return nil
	}

	waitForChild := func() {
		cmd.Wait()
		if cmd.ProcessState != nil {
			fmt.Fprintf(statusW, "%d\n", cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus())
		}
	}

	initChild, childStarted, childEnded, stopAccepting, connected := make(chan bool), make(chan bool), make(chan bool), make(chan bool), make(chan bool)

	onFirstConn := func() {
		connected <- true
		initChild <- true
		<-childStarted
	}

	exitCode := 1
	go acceptConnections(acceptConn, onFirstConn, stopAccepting, processConn)

	select {
	case <-connected:
		<-initChild
		go runChildProcess(startChild, waitForChild, childStarted, childEnded)
		<-childEnded
		exitCode = 0
	case <-time.After(timeout):
		exitCode = 2
		break
	}

	errStream.Close()
	close(stopAccepting)
	listener.Close()
	terminate <- exitCode
}

func acceptConnections(acceptConn func(chan bool) (net.Conn, error), onFirstConn func(), stopAccepting chan bool,
	processConn func(net.Conn)) {
	var once sync.Once

	for {
		conn, err := acceptConn(stopAccepting)
		if err != nil {
			return
		}

		once.Do(onFirstConn)

		processConn(conn)
	}
}

func runChildProcess(startChild func() error, waitForChild func(), childStarted, childEnded chan bool) {
	err := startChild()
	if err != nil {
		return
	}
	childStarted <- true

	waitForChild()
	childEnded <- true
}

func notify(notifyStream io.Writer, message string) {
	fmt.Fprintln(notifyStream, message)
}

func enableTracing(socketPath string, fatal func(error)) {
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

func listen(socketPath string) (net.Listener, error) {
	// Delete socketPath if it exists to avoid bind failures.
	err := os.Remove(socketPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	err = os.MkdirAll(filepath.Dir(socketPath), 0755)
	if err != nil {
		return nil, err
	}

	return net.Listen("unix", socketPath)
}

func acceptConnection(listener net.Listener, stdoutR, stderrR, statusR *os.File) (net.Conn, error) {
	conn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	rights := syscall.UnixRights(
		int(stdoutR.Fd()),
		int(stderrR.Fd()),
		int(statusR.Fd()),
	)

	_, _, err = conn.(*net.UnixConn).WriteMsgUnix([]byte{}, rights, nil)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// Loop receiving and processing link requests on the given connection.
// The loop terminates when the connection is closed or an error occurs.
func processLinkRequests(conn net.Conn, stdinW *os.File, cmd *exec.Cmd, withTty bool) {
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

func createPipes() (stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW *os.File, err error) {
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

func createTtyPty(windowColumns int, windowRows int) (stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW *os.File, err error) {
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
