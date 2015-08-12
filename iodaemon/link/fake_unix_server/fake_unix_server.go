package fake_unix_server

import (
	"net"
	"sync"
)

type FakeUnixServer struct {
	connectionHandler func(net.Conn)
	mutex             *sync.RWMutex
	listener          net.Listener
}

func NewFakeUnixServer(unixSocketPath string) (*FakeUnixServer, error) {
	listener, err := net.Listen("unix", unixSocketPath)
	if err != nil {
		return nil, err
	}

	return &FakeUnixServer{
		connectionHandler: func(conn net.Conn) {
			conn.Close()
		},
		mutex:    new(sync.RWMutex),
		listener: listener,
	}, nil
}

func (s *FakeUnixServer) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}

		s.mutex.RLock()
		go s.connectionHandler(conn)
		s.mutex.RUnlock()
	}

	return nil
}

func (s *FakeUnixServer) SetConnectionHandler(handler func(net.Conn)) {
	s.mutex.Lock()
	s.connectionHandler = handler
	s.mutex.Unlock()
}

func (s *FakeUnixServer) Stop() error {
	return s.listener.Close()
}
