package link

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"

	"github.com/pivotal-golang/lager"
)

type SignalMsg struct {
	Signal syscall.Signal `json:"signal"`
}

type Link struct {
	*Writer

	exitStatus io.ReadCloser
	done       <-chan struct{}
}

func Create(logger lager.Logger, socketPath string, stdout io.Writer, stderr io.Writer) (*Link, error) {
	logger.Info("link-dialing-socket", lager.Data{"socket-path": socketPath})
	conn, err := net.Dial("unix", socketPath)
	logger.Info("link-dialed-socket", lager.Data{"socket-path": socketPath})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to i/o daemon: %s", err)
	}

	var b [2048]byte
	var oob [2048]byte

	logger.Info("read-msg-unix", lager.Data{"socket-path": socketPath})
	n, oobn, _, _, err := conn.(*net.UnixConn).ReadMsgUnix(b[:], oob[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read unix msg: %s (read: %d, %d)", err, n, oobn)
	}

	logger.Info("parse-socket-ctl-message", lager.Data{"socket-path": socketPath})
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, fmt.Errorf("failed to parse socket control message: %s", err)
	}

	if len(scms) < 1 {
		return nil, fmt.Errorf("no socket control messages sent")
	}

	scm := scms[0]

	logger.Info("parse-unix-rights", lager.Data{"socket-path": socketPath})
	fds, err := syscall.ParseUnixRights(&scm)
	if err != nil {
		return nil, fmt.Errorf("failed to parse unix rights: %s", err)
	}

	if len(fds) != 3 {
		return nil, fmt.Errorf("invalid number of fds; need 3, got %d", len(fds))
	}

	logger.Info("close-fds", lager.Data{"socket-path": socketPath})
	for _, fd := range fds {
		logger.Info("close-fd", lager.Data{"socket-path": socketPath, "fd": fd})
		syscall.CloseOnExec(fd)
	}

	logger.Info("create-fds", lager.Data{"socket-path": socketPath})
	lstdout := os.NewFile(uintptr(fds[0]), "stdout")
	lstderr := os.NewFile(uintptr(fds[1]), "stderr")
	lstatus := os.NewFile(uintptr(fds[2]), "status")

	streaming := &sync.WaitGroup{}

	linkWriter := NewWriter(conn)

	logger.Info("copying-stdout", lager.Data{"socket-path": socketPath})
	streaming.Add(1)
	go func() {
		io.Copy(stdout, lstdout)
		lstdout.Close()
		streaming.Done()
	}()

	logger.Info("copying-stderr", lager.Data{"socket-path": socketPath})
	streaming.Add(1)
	go func() {
		io.Copy(stderr, lstderr)
		lstderr.Close()
		streaming.Done()
	}()

	logger.Info("wait-on-streaming", lager.Data{"socket-path": socketPath})
	done := make(chan struct{})
	go func() {
		streaming.Wait()
		close(done)
		conn.Close()
	}()

	return &Link{
		Writer: linkWriter,

		exitStatus: lstatus,
		done:       done,
	}, nil
}

func (link *Link) Wait() (int, error) {
	<-link.done
	defer link.exitStatus.Close()

	var exitStatus int
	_, err := fmt.Fscanf(link.exitStatus, "%d\n", &exitStatus)
	if err != nil {
		return -1, fmt.Errorf("could not determine exit status: %s", err)
	}

	return exitStatus, nil
}
