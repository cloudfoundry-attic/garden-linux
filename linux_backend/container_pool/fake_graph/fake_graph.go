package fake_graph

import (
	"sync"

	"github.com/docker/docker/archive"
	"github.com/docker/docker/image"
)

type FakeGraph struct {
	exists map[string]bool

	WhenRegistering func(image *image.Image, imageJSON []byte, layer archive.ArchiveReader) error

	mutex *sync.RWMutex
}

func New() *FakeGraph {
	return &FakeGraph{
		exists: make(map[string]bool),

		mutex: &sync.RWMutex{},
	}
}

func (graph *FakeGraph) Exists(imageID string) bool {
	graph.mutex.RLock()
	defer graph.mutex.RUnlock()

	return graph.exists[imageID]
}

func (graph *FakeGraph) SetExists(imageID string, exists bool) {
	graph.mutex.Lock()
	graph.exists[imageID] = exists
	graph.mutex.Unlock()
}

func (graph *FakeGraph) Register(image *image.Image, imageJSON []byte, layer archive.ArchiveReader) error {
	if graph.WhenRegistering != nil {
		return graph.WhenRegistering(image, imageJSON, layer)
	}

	return nil
}
