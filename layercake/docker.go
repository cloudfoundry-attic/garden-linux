// Abstracts a layered filesystem provider, such as docker's Graph
package layercake

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/docker/docker/graph"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
)

type Docker struct {
	Graph *graph.Graph
}

func (d *Docker) DriverName() string {
	return d.Graph.Driver().String()
}

func (d *Docker) Create(containerID ID, imageID ID) error {
	return d.Register(
		&image.Image{
			ID:     containerID.GraphID(),
			Parent: imageID.GraphID(),
		}, nil)
}

func (d *Docker) Register(image *image.Image, layer archive.ArchiveReader) error {
	return d.Graph.Register(image, layer)
}

func (d *Docker) Get(id ID) (*image.Image, error) {
	return d.Graph.Get(id.GraphID())
}

func (d *Docker) Remove(id ID) error {
	return d.Graph.Delete(id.GraphID())
}

func (d *Docker) Path(id ID) (string, error) {
	return d.Graph.Driver().Get(id.GraphID(), "")
}

func (d *Docker) IsLeaf(id ID) (bool, error) {
	heads, err := d.Graph.Heads()
	if err != nil {
		return false, err
	}

	_, ok := heads[id.GraphID()]
	return ok, nil
}

type ContainerID string
type DockerImageID string

type LocalImageID struct {
	Path         string
	ModifiedTime time.Time
}

type NamespacedLayerID struct {
	LayerID  string
	CacheKey string
}

func (c ContainerID) GraphID() string {
	return shaID(string(c))
}

func (d DockerImageID) GraphID() string {
	return string(d)
}

func (c LocalImageID) GraphID() string {
	return shaID(fmt.Sprintf("%s-%d", c.Path, c.ModifiedTime))
}

func (n NamespacedLayerID) GraphID() string {
	return shaID(n.LayerID + "@" + n.CacheKey)
}

func shaID(id string) string {
	if id == "" {
		return id
	}

	return fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
}
