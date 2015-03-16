package container_repository

import (
	"sync"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
)

type InMemoryContainerRepository struct {
	store map[string]linux_backend.Container
	mutex *sync.RWMutex
}

func New() *InMemoryContainerRepository {
	return &InMemoryContainerRepository{
		store: map[string]linux_backend.Container{},
		mutex: &sync.RWMutex{},
	}
}

func (cr *InMemoryContainerRepository) All() []linux_backend.Container {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	return cr.Query(func(c linux_backend.Container) bool {
		return true
	})
}

func (cr *InMemoryContainerRepository) Add(container linux_backend.Container) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	cr.store[container.Handle()] = container
}

func (cr *InMemoryContainerRepository) FindByHandle(handle string) (linux_backend.Container, error) {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	container, ok := cr.store[handle]
	if !ok {
		return nil, garden.ContainerNotFoundError{handle}
	}

	return container, nil
}

func (cr *InMemoryContainerRepository) Delete(container linux_backend.Container) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	delete(cr.store, container.Handle())
}

func (cr *InMemoryContainerRepository) Query(filter func(linux_backend.Container) bool) []linux_backend.Container {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	var matches []linux_backend.Container
	for _, c := range cr.store {
		if filter(c) {
			matches = append(matches, c)
		}
	}

	return matches
}
