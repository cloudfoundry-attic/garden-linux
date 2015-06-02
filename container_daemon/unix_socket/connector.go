package unix_socket

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"syscall"
)

type Connector struct {
	SocketPath string
}

type Fd interface {
	io.ReadWriteCloser
	Fd() uintptr
}

func (c *Connector) Connect(msg interface{}) ([]Fd, int, error) {
	conn, err := net.Dial("unix", c.SocketPath)
	if err != nil {
		return nil, 0, fmt.Errorf("unix_socket: connect to server socket: %s", err)
	}
	defer conn.Close() // Ignore error

	msgJson, err := json.Marshal(msg)
	if err != nil {
		return nil, 0, fmt.Errorf("unix_socket: failed to marshal json message: %s", err)
	}

	_, err = conn.Write(msgJson)
	if err != nil {
		return nil, 0, fmt.Errorf("unix_socket: failed to write to connection: %s", err)
	}

	return readData(conn)
}

func readData(conn net.Conn) ([]Fd, int, error) {
	var b [2048]byte
	var oob [2048]byte
	var response Response

	n, oobn, _, _, err := conn.(*net.UnixConn).ReadMsgUnix(b[:], oob[:])
	if err != nil {
		return nil, 0, fmt.Errorf("unix_socket: failed to read unix msg: %s (read: %d, %d)", err, n, oobn)
	}
	logFile, _ := ioutil.TempFile("/tmp", "connector.log")
	fmt.Fprintf(logFile, "ReadMsgUnix returned %d, %d\n", n, oobn)

	if n > 0 {
		err := json.Unmarshal(b[:n], &response)
		if err != nil {
			return nil, 0, fmt.Errorf("unix_socket: Unmarshal failed: %s", err)
		}

		if response.ErrMessage != "" {
			return nil, 0, errors.New(response.ErrMessage)
		}
	} else {
		return nil, 0, errors.New("unix_socket: No response received")
	}

	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, 0, fmt.Errorf("unix_socket: failed to parse socket control message: %s", err)
	}

	if len(scms) < 1 {
		return nil, 0, fmt.Errorf("unix_socket: no socket control messages sent")
	}

	scm := scms[0]
	fds, err := syscall.ParseUnixRights(&scm)
	if err != nil {
		return nil, 0, fmt.Errorf("unix_socket: failed to parse unix rights: %s", err)
	}

	files := make([]Fd, len(fds))
	for i, fd := range fds {
		files[i] = os.NewFile(uintptr(fd), fmt.Sprintf("/dev/fake-fd-%d", i))
	}

	return files, response.Pid, nil
}
