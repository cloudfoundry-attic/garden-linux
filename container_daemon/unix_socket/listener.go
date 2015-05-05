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
	Handle(decoder *json.Decoder) ([]*os.File, int, error)
}

type Listener struct {
	SocketPath   string
	runningMutex sync.RWMutex
	running      bool
	listener     net.Listener
}

type Response struct {
	ErrMessage string
	Pid        int
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
			defer conn.Close() // Ignore error

			decoder := json.NewDecoder(conn)

			files, pid, err := ch.Handle(decoder)
			writeData(conn, files, pid, err)
		}(conn.(*net.UnixConn), ch)
	}
}

func writeData(conn *net.UnixConn, files []*os.File, pid int, responseErr error) {
	var errMsg string = ""
	if responseErr != nil {
		errMsg = responseErr.Error()
	}
	response := &Response{
		Pid:        pid,
		ErrMessage: errMsg,
	}

	responseJson, _ := json.Marshal(response) // Ignore error

	args := make([]int, len(files))
	for i, f := range files {
		args[i] = int(f.Fd())
	}
	resp := syscall.UnixRights(args...)

	conn.WriteMsgUnix(responseJson, resp, nil) // Ignore error
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
