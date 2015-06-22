package rootfs_provider

import (
	"fmt"
	"os"
	"path/filepath"
)

type VolumeCreator interface {
	Create(root string, volume string) error
}

// SimpleVolumeCreator implements volume creation by (simply) creating the
// relevant directories.  If a directory already exists in the image it is
// emptied.
type SimpleVolumeCreator struct{}

func (SimpleVolumeCreator) Create(root string, volume string) error {
	volumePath := filepath.Join(root, volume)

	if info, err := os.Stat(volumePath); err == nil && !info.IsDir() {
		return fmt.Errorf("volume creator: existing file at mount point: %v", volume)
	}

	if err := os.MkdirAll(volumePath, 0755); err != nil {
		return fmt.Errorf("volume creator: creating volume directory: %v", err)
	}

	return nil
}
