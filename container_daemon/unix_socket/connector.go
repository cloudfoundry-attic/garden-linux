package unix_socket

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
)

type Connector struct {
	SocketPath string
}

func (c *Connector) Connect(msg interface{}) ([]io.ReadWriteCloser, error) {
	conn, err := net.Dial("unix", c.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("unix_socket: connect to server socket: %s", err)
	}
	defer conn.Close()

	msgJson, _ := json.Marshal(msg)
	conn.Write(msgJson)

	var b [2048]byte
	var oob [2048]byte
	n, oobn, _, _, err := conn.(*net.UnixConn).ReadMsgUnix(b[:], oob[:])
	if err != nil {
		return nil, fmt.Errorf("unix_socket: failed to read unix msg: %s (read: %d, %d)", err, n, oobn)
	}

	if n > 1 {
		return nil, fmt.Errorf("%s", string(b[:n]))
	}

	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, fmt.Errorf("unix_socket: failed to parse socket control message: %s", err)
	}

	if len(scms) < 1 {
		return nil, fmt.Errorf("unix_socket: no socket control messages sent")
	}

	scm := scms[0]
	fds, err := syscall.ParseUnixRights(&scm)
	if err != nil {
		return nil, fmt.Errorf("unix_socket: failed to parse unix rights: %s", err)
	}

	res := make([]io.ReadWriteCloser, len(fds))
	for i, fd := range fds {
		res[i] = os.NewFile(uintptr(fd), fmt.Sprintf("/dev/fake-fd-%d", i))
	}

	return res, nil
}
