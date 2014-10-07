package fake_graph

import (
	"errors"
	"sync"

	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
)

type FakeGraph struct {
	exists map[string]*image.Image

	WhenRegistering func(image *image.Image, imageJSON []byte, layer archive.ArchiveReader) error

	mutex *sync.RWMutex
}

func New() *FakeGraph {
	return &FakeGraph{
		exists: make(map[string]*image.Image),

		mutex: &sync.RWMutex{},
	}
}

func (graph *FakeGraph) Exists(imageID string) bool {
	graph.mutex.RLock()
	defer graph.mutex.RUnlock()

	_, exists := graph.exists[imageID]
	return exists
}

func (graph *FakeGraph) SetExists(imageID string, imgJSON []byte) {
	graph.mutex.Lock()
	defer graph.mutex.Unlock()
	img, err := image.NewImgJSON(imgJSON)
	if err != nil {
		panic("bad imgJSON for imageID: " + imageID)
	}
	graph.exists[imageID] = img
}

func (graph *FakeGraph) Get(imageID string) (*image.Image, error) {
	graph.mutex.RLock()
	defer graph.mutex.RUnlock()

	img, exists := graph.exists[imageID]
	if exists {
		return img, nil
	} else {
		return nil, errors.New("image not found in graph: " + imageID)
	}
}

func (graph *FakeGraph) Register(image *image.Image, imageJSON []byte, layer archive.ArchiveReader) error {
	if graph.WhenRegistering != nil {
		return graph.WhenRegistering(image, imageJSON, layer)
	}

	return nil
}
