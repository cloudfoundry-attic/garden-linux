package rootfs_provider

import (
	"github.com/docker/docker/daemon/graphdriver"
	"github.com/pivotal-golang/lager"
)

type GraphCleaner struct {
	GraphDriver graphdriver.Driver
}

func (c *GraphCleaner) Clean(logger lager.Logger, id string) error {
	c.GraphDriver.Put(id)
	return c.GraphDriver.Remove(id)
}
