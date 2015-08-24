package rootfs_provider

import (
	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/image"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_graph/fake_graph.go . Graph
type Graph interface {
	Get(id string) (*image.Image, error)
	IsParent(id string) bool
}

type GraphCleaner struct {
	Graph       Graph
	GraphDriver graphdriver.Driver
}

func (c *GraphCleaner) Clean(logger lager.Logger, id string) error {
	c.GraphDriver.Put(id)
	image, _ := c.Graph.Get(id)

	if err := c.GraphDriver.Remove(id); err != nil {
		return err
	}

	if image.Parent == "" || c.Graph.IsParent(image.Parent) {
		return nil
	}

	return c.Clean(logger, image.Parent)
}
