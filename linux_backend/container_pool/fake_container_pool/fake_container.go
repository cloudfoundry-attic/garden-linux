package fake_container_pool

import (
	"io"
	"sync"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/garden/warden/fake_backend"
)

type FakeContainer struct {
	*fake_backend.FakeContainer

	SnapshotError  error
	SavedSnapshots []io.Writer
	snapshotMutex  *sync.RWMutex
}

func NewFakeContainer(spec warden.ContainerSpec) *FakeContainer {
	return &FakeContainer{
		FakeContainer: fake_backend.NewFakeContainer(spec),

		snapshotMutex: new(sync.RWMutex),
	}
}

func (c *FakeContainer) Snapshot(snapshot io.Writer) error {
	if c.SnapshotError != nil {
		return c.SnapshotError
	}

	c.snapshotMutex.Lock()
	defer c.snapshotMutex.Unlock()

	c.SavedSnapshots = append(c.SavedSnapshots, snapshot)

	return nil
}
