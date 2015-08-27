package layercake

import (
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
)

//go:generate counterfeiter -o fake_cake/fake_cake.go . Cake
type Cake interface {
	DriverName() string
	Create(containerID, imageID IDer) error
	Register(img *image.Image, layer archive.ArchiveReader) error
	Get(id IDer) (*image.Image, error)
	Remove(id IDer) error
	Path(id IDer) (string, error)
}
