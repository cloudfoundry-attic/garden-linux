package container_repository

import (
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend"
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

	all := []linux_backend.Container{}
	for _, container := range cr.store {
		all = append(all, container)
	}
	return all
}

func (cr *InMemoryContainerRepository) Add(container linux_backend.Container) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	cr.store[container.Handle()] = container
}

func (cr *InMemoryContainerRepository) FindByHandle(handle string) (linux_backend.Container, bool) {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	// Yep, you actually can't inline this...
	container, ok := cr.store[handle]
	return container, ok
}

func (cr *InMemoryContainerRepository) Delete(container linux_backend.Container) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	delete(cr.store, container.Handle())
}
