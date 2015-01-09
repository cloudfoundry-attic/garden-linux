package linux_backend

import (
	"encoding/json"
	"time"

	"github.com/cloudfoundry-incubator/garden"
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
	Network *json.RawMessage
	Ports   []uint32
}

type ProcessSnapshot struct {
	ID  uint32
	TTY bool
}
