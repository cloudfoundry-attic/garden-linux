package gordon

import (
	"github.com/cloudfoundry-incubator/gordon/connection"
)

type ConnectionProvider interface {
	ProvideConnection() (*connection.Connection, error)
}

type ConnectionInfo struct {
	Network string
	Addr    string
}

func (i *ConnectionInfo) ProvideConnection() (*connection.Connection, error) {
	return connection.Connect(i.Network, i.Addr)
}
