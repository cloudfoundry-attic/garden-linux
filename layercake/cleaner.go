package layercake

import (
	"sync"

	"github.com/docker/docker/image"

	"github.com/pivotal-golang/lager"
)

type OvenCleaner struct {
	Cake

	Logger lager.Logger

	EnableImageCleanup bool

	images   map[string]int
	imagesMu sync.RWMutex

	mu map[ID]*sync.RWMutex
}

func (g *OvenCleaner) Get(id ID) (*image.Image, error) {
	g.l(id).RLock() // avoid saying image is there if we might be in the process of deleting it
	defer g.l(id).RUnlock()

	return g.Cake.Get(id)
}

func (g *OvenCleaner) Remove(id ID) error {
	g.l(id).Lock()
	defer g.l(id).Unlock()

	log := g.Logger.Session("remove", lager.Data{"ID": id})
	log.Info("start")

	if g.isHeld(id) {
		log.Info("layer-is-held")
		return nil
	}

	img, err := g.Cake.Get(id)
	if err != nil {
		log.Error("get-image", err)
		return nil
	}

	if err := g.Cake.Remove(id); err != nil {
		return err
	}

	if !g.EnableImageCleanup {
		return nil
	}

	if img.Parent == "" {
		return nil
	}
	if leaf, err := g.Cake.IsLeaf(DockerImageID(img.Parent)); err == nil && leaf {
		return g.Remove(DockerImageID(img.Parent))
	}

	return nil
}

func (g *OvenCleaner) Retain(id ID) {
	g.imagesMu.Lock()
	defer g.imagesMu.Unlock()

	if g.images == nil {
		g.images = make(map[string]int)
	}

	g.images[id.GraphID()]++
}

func (g *OvenCleaner) isHeld(id ID) bool {
	g.imagesMu.Lock()
	defer g.imagesMu.Unlock()

	if g.images == nil {
		g.images = make(map[string]int)
	}

	_, ok := g.images[id.GraphID()]
	return ok
}

func (g *OvenCleaner) l(id ID) *sync.RWMutex {
	if g.mu == nil {
		g.mu = make(map[ID]*sync.RWMutex)
	}

	if m, ok := g.mu[id]; ok {
		return m
	}

	g.mu[id] = &sync.RWMutex{}
	return g.mu[id]
}
