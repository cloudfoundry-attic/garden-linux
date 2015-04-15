package container_daemon

import (
	"fmt"
	"net"
	"sync"
)

type ContainerDaemon struct {
	SocketPath string
	running    bool
	stateMutex sync.RWMutex
	listener   net.Listener
}

func (cd *ContainerDaemon) Init() error {
	var err error

	cd.listener, err = net.Listen("unix", cd.SocketPath)
	if err != nil {
		return fmt.Errorf("container_daemon: error creating socket: %v", err)
	}

	return nil
}

func (cd *ContainerDaemon) Run() error {
	cd.setRunning(true)

	var conn net.Conn
	var err error

	for {
		conn, err = cd.listener.Accept()
		if !cd.isRunning() {
			return nil
		}

		if err != nil {
			return fmt.Errorf("container_daemon: Failure while accepting: %v", err)
		}

		conn.Write([]byte("Accepting connections"))
		conn.Close()
	}

	return nil
}

func (cd *ContainerDaemon) Stop() error {
	cd.setRunning(false)
	return cd.listener.Close()
}

func (cd *ContainerDaemon) setRunning(running bool) {
	cd.stateMutex.Lock()
	cd.running = running
	cd.stateMutex.Unlock()
}

func (cd *ContainerDaemon) isRunning() bool {
	cd.stateMutex.RLock()
	defer cd.stateMutex.RUnlock()
	return cd.running
}
