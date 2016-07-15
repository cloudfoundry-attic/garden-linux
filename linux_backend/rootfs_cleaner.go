package linux_backend

import (
	"os"
	"path/filepath"

	"code.cloudfoundry.org/lager"
)

type RootFSCleaner struct {
	FilePaths []string
}

func (r *RootFSCleaner) Clean(log lager.Logger, path string) error {
	log = log.Session("rootfs-cleaner", lager.Data{"path": path})

	for _, filePath := range r.FilePaths {
		filePath = filepath.Join(path, filePath)
		fi, err := os.Lstat(filePath)
		if os.IsNotExist(err) {
			continue
		}

		if fi.Mode()&os.ModeSymlink != 0 {
			err := os.Remove(filePath)
			if err != nil {
				log.Error("symlink-remove-failed", err)
				return err
			}
		}
	}

	return nil
}
