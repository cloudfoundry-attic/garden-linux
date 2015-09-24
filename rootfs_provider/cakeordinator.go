package rootfs_provider

import (
	"net/url"
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
)

//go:generate counterfeiter . LayerCreator
type LayerCreator interface {
	Create(id string, parentImage *repository_fetcher.Image, shouldNamespace bool, quota int64) (string, process.Env, error)
}

//go:generate counterfeiter . RepositoryFetcher
type RepositoryFetcher interface {
	Fetch(*url.URL, int64) (*repository_fetcher.Image, error)
}

// CakeOrdinator manages a cake, fetching layers as neccesary
type CakeOrdinator struct {
	mu           sync.RWMutex
	cake         layercake.Cake
	fetcher      RepositoryFetcher
	layerCreator LayerCreator
}

// New creates a new cake-ordinator, there should only be one CakeOrdinator
// for a particular cake.
func NewCakeOrdinator(cake layercake.Cake, fetcher RepositoryFetcher, layerCreator LayerCreator) *CakeOrdinator {
	return &CakeOrdinator{cake: cake, fetcher: fetcher, layerCreator: layerCreator}
}

func (c *CakeOrdinator) Create(id string, parentImageURL *url.URL, translateUIDs bool, diskQuota int64) (string, process.Env, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	image, err := c.fetcher.Fetch(parentImageURL, diskQuota)
	if err != nil {
		return "", nil, err
	}

	return c.layerCreator.Create(id, image, translateUIDs, diskQuota)
}

func (c *CakeOrdinator) Remove(id layercake.ID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.cake.Remove(id)
}
