package rootfs_provider

import (
	"crypto/sha256"
	"fmt"

	"github.com/docker/docker/graph"
	"github.com/docker/docker/image"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_graph/fake_graph.go . Graph
type Graph interface {
	Get(id string) (*image.Image, error)
	ByParent() (map[string][]*image.Image, error)
}

type GraphCleaner struct {
	Graph   Graph
	Cleaner RootFSCleaner
}

func (c *GraphCleaner) Clean(logger lager.Logger, id string) error {
	logger = logger.Session("clean-graph", lager.Data{"id": id})

	if len(id) < 20 {
		id = fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
	}

	image, err := c.Graph.Get(id)
	if err != nil {
		logger.Error("get", err)
		return fmt.Errorf("clean graph: %s", err)
	}

	if err := c.Cleaner.Clean(logger, id); err != nil {
		logger.Error("remove", err)
		return fmt.Errorf("clean graph: %s", err)
	}

	byParent, _ := c.Graph.ByParent()
	if _, isParent := byParent[image.Parent]; image.Parent == "" || isParent {
		return nil
	}

	return c.Clean(logger, image.Parent)
}

type GraphLayerCleaner struct {
	Graph *graph.Graph
}

func (g *GraphLayerCleaner) Clean(logger lager.Logger, id string) error {
	return g.Graph.Delete(id)
}
