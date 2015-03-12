package linux_backend

import "sync"

type InMemoryContainerRepository struct {
	store map[string]Container
	mutex *sync.RWMutex
}

func (cr *InMemoryContainerRepository) All() []Container {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	all := []Container{}
	for _, container := range cr.store {
		all = append(all, container)
	}
	return all
}

func (cr *InMemoryContainerRepository) Add(container Container) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	cr.store[container.Handle()] = container
}

func (cr *InMemoryContainerRepository) FindByHandle(handle string) (Container, bool) {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	// Yep, you actually can't inline these...
	container, ok := cr.store[handle]
	return container, ok
}

func (cr *InMemoryContainerRepository) Delete(container Container) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	delete(cr.store, container.Handle())
}
