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
	return d.Driver.Get(id.GraphID(), "")
}

type ID interface {
	GraphID() string
}

type ContainerID string
type DockerImageID string

func (c ContainerID) GraphID() string {
	return shaID(string(c))
}

func (d DockerImageID) GraphID() string {
	return string(d)
}

func shaID(id string) string {
	if id == "" {
		return id
	}

	return fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
}
