// Abstracts a layered filesystem provider, such as docker's Graph
package layercake

import (
	"crypto/sha256"
	"fmt"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/graph"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
)

type Docker struct {
	Graph  *graph.Graph
	Driver graphdriver.Driver
}

func (d *Docker) DriverName() string {
	return d.Driver.String()
}

func (d *Docker) Create(containerID IDer, imageID IDer) error {
	return d.Register(
		&image.Image{
			ID:     containerID.ID(),
			Parent: imageID.ID(),
		}, nil)
}

func (d *Docker) Register(image *image.Image, layer archive.ArchiveReader) error {
	return d.Graph.Register(image, layer)
}

func (d *Docker) Get(id IDer) (*image.Image, error) {
	return d.Graph.Get(id.ID())
}

func (d *Docker) Remove(id IDer) error {
	return d.Graph.Delete(id.ID())
}

func (d *Docker) Path(id IDer) (string, error) {
	return d.Driver.Get(id.ID(), "")
}

type IDer interface {
	ID() string
}

type ContainerID string
type DockerImageID string

func (c ContainerID) ID() string {
	return shaID(string(c))
}

func (d DockerImageID) ID() string {
	return string(d)
}

func shaID(id string) string {
	if id == "" {
		return id
	}

	return fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
}
