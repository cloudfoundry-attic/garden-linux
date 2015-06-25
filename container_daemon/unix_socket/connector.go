package unix_socket

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
)

type Connector struct {
	SocketPath string
}

func (c *Connector) Connect(msg interface{}) ([]container_daemon.StreamingFile, int, error) {
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

func readData(conn net.Conn) ([]container_daemon.StreamingFile, int, error) {
	var b [2048]byte
	var oob [2048]byte
	var response Response

	n, oobn, _, _, err := conn.(*net.UnixConn).ReadMsgUnix(b[:], oob[:])
	if err != nil {
		return nil, 0, fmt.Errorf("unix_socket: failed to read unix msg: %s (read: %d, %d)", err, n, oobn)
	}

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

	files := make([]container_daemon.StreamingFile, len(fds))
	for i, fd := range fds {
		files[i] = os.NewFile(uintptr(fd), fmt.Sprintf("/dev/fake-fd-%d", i))
	}

	return files, response.Pid, nil
}
