package fake_container_pool

import (
	"fmt"
	"io"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend"
	"github.com/cloudfoundry-incubator/garden/api"
	"github.com/nu7hatch/gouuid"
)

type FakeContainerPool struct {
	DidSetup bool

	MaxContainersValue int

	Pruned         bool
	PruneError     error
	KeptContainers map[string]bool

	CreateError  error
	RestoreError error
	DestroyError error

	ContainerSetup func(*FakeContainer)

	CreatedContainers   []linux_backend.Container
	DestroyedContainers []linux_backend.Container
	RestoredSnapshots   []io.Reader
}

func New() *FakeContainerPool {
	return &FakeContainerPool{}
}

func (p *FakeContainerPool) MaxContainers() int {
	return p.MaxContainersValue
}

func (p *FakeContainerPool) Setup() error {
	p.DidSetup = true

	return nil
}

func (p *FakeContainerPool) Prune(keep map[string]bool) error {
	if p.PruneError != nil {
		return p.PruneError
	}

	p.Pruned = true
	p.KeptContainers = keep

	return nil
}

func (p *FakeContainerPool) Create(spec api.ContainerSpec) (linux_backend.Container, error) {
	if p.CreateError != nil {
		return nil, p.CreateError
	}

	idUUID, err := uuid.NewV4()
	if err != nil {
		panic("could not create uuid: " + err.Error())
	}

	id := idUUID.String()[:11]

	if spec.Handle == "" {
		spec.Handle = id
	}

	container := NewFakeContainer(spec)

	if p.ContainerSetup != nil {
		p.ContainerSetup(container)
	}

	p.CreatedContainers = append(p.CreatedContainers, container)

	return container, nil
}

func (p *FakeContainerPool) Restore(snapshot io.Reader) (linux_backend.Container, error) {
	if p.RestoreError != nil {
		return nil, p.RestoreError
	}

	var handle string

	_, err := fmt.Fscanf(snapshot, "%s", &handle)
	if err != nil && err != io.EOF {
		return nil, err
	}

	container := NewFakeContainer(
		api.ContainerSpec{
			Handle: handle,
		},
	)

	p.RestoredSnapshots = append(p.RestoredSnapshots, snapshot)

	return container, nil
}

func (p *FakeContainerPool) Destroy(container linux_backend.Container) error {
	if p.DestroyError != nil {
		return p.DestroyError
	}

	p.DestroyedContainers = append(p.DestroyedContainers, container)

	return nil
}
