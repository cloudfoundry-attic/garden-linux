package linux_backend

import (
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"

	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/network"
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
	NetOuts []NetOutSpec

	Properties warden.Properties

	EnvVars []string
}

type LimitsSnapshot struct {
	Memory    *warden.MemoryLimits
	Disk      *warden.DiskLimits
	Bandwidth *warden.BandwidthLimits
	CPU       *warden.CPULimits
}

type ResourcesSnapshot struct {
	UID     uint32
	Network *network.Network
	Ports   []uint32
}

type ProcessSnapshot struct {
	ID  uint32
	TTY bool
}
