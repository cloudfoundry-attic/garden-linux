package repository_fetcher

import (
	"net/url"
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
)

// CakeOrdinator manages a cake, fetching layers as neccesary
type CakeOrdinator struct {
	mu      sync.RWMutex
	cake    layercake.Cake
	fetcher RepositoryFetcher
}

// New creates a new cake-ordinator, there should only be one CakeOrdinator
// for a particular cake.
func NewCakeOrdinator(cake layercake.Cake, fetcher RepositoryFetcher) *CakeOrdinator {
	return &CakeOrdinator{cake: cake, fetcher: fetcher}
}

func (c *CakeOrdinator) Fetch(url *url.URL, diskQuota int64) (*Image, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.fetcher.Fetch(url, diskQuota)
}

func (c *CakeOrdinator) Remove(id layercake.ID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.cake.Remove(id)
}
