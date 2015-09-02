package layercake

import "sync"

type registry struct {
	mu sync.RWMutex

	images map[string]int
}

func NewRegistry() *registry {
	return &registry{
		images: make(map[string]int),
	}
}

func (r *registry) Retain(id ID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.images[id.GraphID()]++
}

func (r *registry) Release(id ID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.images[id.GraphID()]--
	if r.images[id.GraphID()] <= 0 {
		delete(r.images, id.GraphID())
	}
}

func (r *registry) IsHeld(id ID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.images[id.GraphID()]
	return ok
}
