package iodaemon

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager"

	"io"
)

// spawn listens on a unix socket at the given socketPath and when the first connection
// is received, starts a child process.
func Spawn(
	logger lager.Logger,
	socketPath string,
	argv []string,
	timeout time.Duration,
	notifyStream io.WriteCloser,

	wirer *Wirer,
	daemon *Daemon,
) error {
	logger = logger.Session("spawn")
	logger.Debug("start", lager.Data{"socketPath": socketPath, "timeout": timeout})
	defer logger.Debug("end")

	logger.Debug("listen-on-socket")
	listener, err := listen(socketPath)
	if err != nil {
		return err
	}

	defer listener.Close()

	logger.Debug("look-up-executable-path")
	executablePath, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("executable %s not found: %s", argv[0], err)
	}

	cmd := child(executablePath, argv)

	logger.Debug("wire-wirer")
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
			logger.Debug("send-ready")
			fmt.Fprintln(notifyStream, "ready")

			logger.Debug("accept-connection")
			conn, err := acceptConnection(listener, stdoutR, stderrR, statusR)
			if err != nil {
				errChan <- err
				return // in general this means the listener has been closed
			}
			logger.Debug("accepted-connection")

			once.Do(func() {
				logger.Debug("start-cmd")
				err := cmd.Start()
				if err != nil {
					errChan <- fmt.Errorf("executable %s failed to start: %s", executablePath, err)
					return
				}

				logger.Debug("send-active")
				fmt.Fprintln(notifyStream, "active")

				logger.Debug("close-notify-stream")
				notifyStream.Close()
				launched <- true
			})

			logger.Debug("handle-connection")
			daemon.HandleConnection(conn, cmd.Process, stdinW, extraFdW)
			logger.Debug("handled-connection")
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-launched:
		logger.Debug("launched-cmd")
		var exit byte = 0
		if err := cmd.Wait(); err != nil {
			ws := err.(*exec.ExitError).ProcessState.Sys().(syscall.WaitStatus)
			exit = byte(ws.ExitStatus())
		}

		fmt.Fprintf(statusW, "%d\n", exit)
	case <-time.After(timeout):
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
