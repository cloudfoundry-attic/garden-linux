package rootfs_provider

import (
	"github.com/docker/docker/daemon/graphdriver"
	"github.com/pivotal-golang/lager"
)

type VfsRootFSRemover struct {
	GraphDriver graphdriver.Driver
}

func (c *VfsRootFSRemover) CleanupRootFS(logger lager.Logger, id string) error {
	c.GraphDriver.Put(id)
	return c.GraphDriver.Remove(id)
}
