package rootfs_provider

import (
	"fmt"

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
	logger = logger.Session("clean-graph", lager.Data{"id": id})

	c.GraphDriver.Put(id)
	image, err := c.Graph.Get(id)
	if err != nil {
		logger.Error("get", err)
		return fmt.Errorf("clean graph: %s", err)
	}

	if err := c.GraphDriver.Remove(id); err != nil {
		logger.Error("remove", err)
		return fmt.Errorf("clean graph: %s", err)
	}

	if image.Parent == "" || c.Graph.IsParent(image.Parent) {
		return nil
	}

	return c.Clean(logger, image.Parent)
}
