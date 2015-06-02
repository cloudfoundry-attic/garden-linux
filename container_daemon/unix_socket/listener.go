package unix_socket

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

//go:generate counterfeiter -o fake_connection_handler/FakeConnectionHandler.go . ConnectionHandler
type ConnectionHandler interface {
	Handle(decoder *json.Decoder) ([]*os.File, int, error)
}

type Listener struct {
	listener net.Listener
}

type Response struct {
	ErrMessage string
	Pid        int
}

// This function should be called from the host namespace, to open the socket file in the right file system.
func NewListenerFromPath(socketPath string) (*Listener, error) {
	l := &Listener{}
	var err error

	l.listener, err = net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("container_daemon: error creating socket: %v", err)
	}

	return l, nil
}

func (l *Listener) Listen(ch ConnectionHandler) error {
	if l.listener == nil {
		return errors.New("unix_socket: listener is not initialized")
	}

	for {
		var conn net.Conn
		var err error
		conn, err = l.listener.Accept()
		if err != nil {
			return fmt.Errorf("container_daemon: Failure while accepting: %v", err)
		}
		//doNothing()
		//checkFd(conn, "After Accept")

		go func(conn *net.UnixConn, ch ConnectionHandler) {
			defer conn.Close() // Ignore error

			decoder := json.NewDecoder(conn)

			files, pid, err := ch.Handle(decoder)
			writeData(conn, files, pid, err)
		}(conn.(*net.UnixConn), ch)
	}
}

//func doNothing() {
//}

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

	//checkFd(conn, "Before write")
	conn.WriteMsgUnix(responseJson, resp, nil) // Ignore error

	// Close the files whose descriptors have been sent to the host to ensure that
	// a close on the host takes effect in a timely fashion.
	for _, file := range files {
		file.Close() // Ignore error
	}
}

func (l *Listener) Close() error {
	return l.listener.Close()
}

//func checkFd(conn net.Conn, msg string) {
//	return
//	unixConn, ok := conn.(*net.UnixConn)
//	if !ok {
//		fmt.Fprintln(os.Stderr, "Wrong connection type")
//		return
//	}
//	connFile, err := unixConn.File()
//	if err != nil {
//		fmt.Fprintf(os.Stderr, "unixConn.File failed: %s\n", err)
//		return
//	}
//	fileInfo, err := connFile.Stat()
//	if err != nil {
//		fmt.Fprintf(os.Stderr, "os.Stat failed: %s\n", err)
//		return
//	}
//	fmt.Fprintf(os.Stderr, "%s: connection stat: %#v\n", msg, fileInfo.Sys())
//}
