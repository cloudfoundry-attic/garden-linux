package unix_socket

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
)

//go:generate counterfeiter -o fake_connection_handler/FakeConnectionHandler.go . ConnectionHandler
type ConnectionHandler interface {
	Handle(decoder *json.Decoder) ([]*os.File, error)
}

type Listener struct {
	SocketPath   string
	runningMutex sync.RWMutex
	running      bool
	listener     net.Listener
}

// This method should be called from the host namespace, to open the socket file in the right file system.
func (l *Listener) Init() error {
	var err error

	l.listener, err = net.Listen("unix", l.SocketPath)
	if err != nil {
		return fmt.Errorf("container_daemon: error creating socket: %v", err)
	}

	return nil
}

func (l *Listener) Listen(ch ConnectionHandler) error {
	if l.listener == nil {
		return errors.New("unix_socket: listener is not initialized")
	}
	l.setRunning(true)

	var conn net.Conn
	var err error
	for {
		conn, err = l.listener.Accept()
		if !l.isRunning() {
			return nil
		}
		if err != nil {
			return fmt.Errorf("container_daemon: Failure while accepting: %v", err)
		}

		go func(conn *net.UnixConn, ch ConnectionHandler) {
			defer conn.Close()

			decoder := json.NewDecoder(conn)

			files, err := ch.Handle(decoder)
			if err != nil {
				conn.Write([]byte(err.Error()))
				return
			}

			args := make([]int, len(files))
			for i, f := range files {
				args[i] = int(f.Fd())
			}
			resp := syscall.UnixRights(args...)
			_, _, err = conn.WriteMsgUnix([]byte{}, resp, nil)
			if err != nil {
				// TODO: Send back the error
			}
		}(conn.(*net.UnixConn), ch)
	}

	return nil
}

func (l *Listener) Stop() error {
	l.setRunning(false)
	return l.listener.Close()
}

func (l *Listener) setRunning(running bool) {
	l.runningMutex.Lock()
	l.running = running
	l.runningMutex.Unlock()
}

func (l *Listener) isRunning() bool {
	l.runningMutex.RLock()
	defer l.runningMutex.RUnlock()
	return l.running
}
