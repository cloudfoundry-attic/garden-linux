package rootfs_provider

import (
	"net/url"
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter . LayerCreator
type LayerCreator interface {
	Create(id string, parentImage *repository_fetcher.Image, shouldNamespace bool, quota int64) (string, process.Env, error)
}

//go:generate counterfeiter . RepositoryFetcher
type RepositoryFetcher interface {
	Fetch(*url.URL, int64) (*repository_fetcher.Image, error)
	FetchID(*url.URL) (layercake.ID, error)
}

// CakeOrdinator manages a cake, fetching layers as neccesary
type CakeOrdinator struct {
	mu           sync.RWMutex
	cake         layercake.Cake
	fetcher      RepositoryFetcher
	layerCreator LayerCreator
	logger       lager.Logger
	whiteList    []layercake.ID
}

// New creates a new cake-ordinator, there should only be one CakeOrdinator
// for a particular cake.
func NewCakeOrdinator(cake layercake.Cake, fetcher RepositoryFetcher, layerCreator LayerCreator, logger lager.Logger) *CakeOrdinator {
	return &CakeOrdinator{
		cake:         cake,
		fetcher:      fetcher,
		layerCreator: layerCreator,
		logger:       logger}
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

func (c *CakeOrdinator) WhiteList(images []string) {
	for _, image := range images {
		imageURL, err := url.Parse(image)
		if err != nil {
			c.logger.Error("url.Parse", err, lager.Data{"image": image})
			continue
		}
		id, err := c.fetcher.FetchID(imageURL)
		if err != nil {
			c.logger.Error("RepositoryFetcher.FetchID", err, lager.Data{"image": image})
			continue
		}
		c.whiteList = append(c.whiteList, id)
	}
}

func (c *CakeOrdinator) Remove(id layercake.ID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	session := c.logger.Session("Remove", lager.Data{"id": id})

	for _, whiteID := range c.whiteList {
		if whiteID == id {
			session.Info("Skipping whitelisted image")
			return nil
		}
	}

	return c.cake.Remove(id)
}
