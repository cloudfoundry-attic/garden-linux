package iodaemon

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"io"
)

var (
	logFile     *os.File
	multiWriter io.Writer
)

// spawn listens on a unix socket at the given socketPath and when the first connection
// is received, starts a child process.
func Spawn(
	socketPath string,
	argv []string,
	timeout time.Duration,
	notifyStream io.WriteCloser,

	wirer *Wirer,
	daemon *Daemon,
) error {
	listener, err := listen(socketPath)
	if err != nil {
		return err
	}

	pid := strings.Split(filepath.Base(socketPath), ".")[0]
	logFile, err = os.OpenFile(filepath.Join(filepath.Dir(socketPath), fmt.Sprintf(pid+".iodaemon")), os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		panic(err)
	}

	multiWriter = io.MultiWriter(logFile, notifyStream)

	defer listener.Close()

	executablePath, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("executable %s not found: %s", argv[0], err)
	}

	cmd := child(executablePath, argv)

	stdinW, stdoutR, stderrR, extraFdW, err := wirer.Wire(cmd)
	if err != nil {
		return err
	}

	statusR, statusW, err := os.Pipe()
	if err != nil {
		return err
	}

	launched := make(chan bool)
	errChan := make(chan error)
	go func() {
		var once sync.Once

		for {
			fmt.Fprintln(multiWriter, "ready")
			conn, err := acceptConnection(multiWriter, listener, stdoutR, stderrR, statusR)
			fmt.Fprintln(multiWriter, "accepted-connection")
			if err != nil {
				errChan <- err
				return // in general this means the listener has been closed
			}

			once.Do(func() {
				fmt.Fprintln(multiWriter, "cmd-start")
				err := cmd.Start()
				fmt.Fprintln(multiWriter, "cmd-started")
				if err != nil {
					errChan <- fmt.Errorf("executable %s failed to start: %s", executablePath, err)
					return
				}

				fmt.Fprintln(multiWriter, "active")
				// multiWriter.Close()
				launched <- true
			})

			daemon.HandleConnection(conn, cmd.Process, stdinW, extraFdW)
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-launched:
		var exit byte = 0
		if err := cmd.Wait(); err != nil {
			ws := err.(*exec.ExitError).ProcessState.Sys().(syscall.WaitStatus)
			exit = byte(ws.ExitStatus())
		}

		fmt.Fprintf(statusW, "%d\n", exit)
	case <-time.After(timeout):
		contents, err := ioutil.ReadFile("/proc/net/unix")
		if err != nil {
			fmt.Fprintf(multiWriter, "Failed to open /proc/net/unix: %s\n", err)
		} else {
			fmt.Fprintf(logFile, "/proc/net/unix = `%s`\n", string(contents))
		}

		fmt.Fprintf(logFile, "output of netstat -xap: \n")
		cmd := exec.Command("netstat", "-xap")
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(multiWriter, "Failed to run netstat: %s\n", err)
		}

		return fmt.Errorf("expected client to connect within %s", timeout)
	}

	return nil
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

func acceptConnection(multiWriter io.Writer, listener net.Listener, stdoutR, stderrR, statusR *os.File) (net.Conn, error) {
	conn, err := listener.Accept()
	fmt.Fprintln(multiWriter, "listener-accepted")
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(multiWriter, "unix-rights")
	rights := syscall.UnixRights(
		int(stdoutR.Fd()),
		int(stderrR.Fd()),
		int(statusR.Fd()),
	)

	fmt.Fprintln(multiWriter, "write-msg-unix")
	_, _, err = conn.(*net.UnixConn).WriteMsgUnix([]byte{}, rights, nil)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
