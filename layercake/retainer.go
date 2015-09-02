package layercake

import "sync"

//go:generate counterfeiter -o fake_retainer/fake_retainer.go . Retainer
type Retainer interface {
	Retain(id ID)
	Release(id ID)
	IsHeld(id ID) bool
}

type retainer struct {
	mu     sync.RWMutex
	images map[string]int
}

func NewRetainer() *retainer {
	return &retainer{
		images: make(map[string]int),
	}
}

func (r *retainer) Retain(id ID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.images[id.GraphID()]++
}

func (r *retainer) Release(id ID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.images[id.GraphID()]--
	if r.images[id.GraphID()] <= 0 {
		delete(r.images, id.GraphID())
	}
}

func (r *retainer) IsHeld(id ID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.images[id.GraphID()]
	return ok
}
