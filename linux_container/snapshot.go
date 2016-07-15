package linux_container

import (
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden-linux/linux_backend"
)

type ContainerSnapshot struct {
	ID         string
	Handle     string
	RootFSPath string

	GraceTime time.Duration

	State  string
	Events []string

	Limits linux_backend.Limits

	Resources ResourcesSnapshot

	Processes               []linux_backend.ActiveProcess
	DefaultProcessSignaller bool

	NetIns  []linux_backend.NetInSpec
	NetOuts []garden.NetOutRule

	Properties garden.Properties

	EnvVars []string
}

type ResourcesSnapshot struct {
	UserUID int
	RootUID int
	Network *linux_backend.Network
	Bridge  string
	Ports   []uint32
}
