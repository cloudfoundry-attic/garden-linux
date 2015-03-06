package linux_container

import (
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend"
)

type ContainerSnapshot struct {
	ID     string
	Handle string

	GraceTime time.Duration

	State  string
	Events []string

	Limits LimitsSnapshot

	Resources ResourcesSnapshot

	Processes []ProcessSnapshot

	NetIns  []NetInSpec
	NetOuts []garden.NetOutRule

	Properties garden.Properties

	EnvVars []string
}

type LimitsSnapshot struct {
	Memory    *garden.MemoryLimits
	Disk      *garden.DiskLimits
	Bandwidth *garden.BandwidthLimits
	CPU       *garden.CPULimits
}

type ResourcesSnapshot struct {
	UserUID uint32
	RootUID uint32
	Network *linux_backend.Network
	Bridge  string
	Ports   []uint32
}

type ProcessSnapshot struct {
	ID  uint32
	TTY bool
}
