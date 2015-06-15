package legacy_aufs_remover

import (
	"os"
	"path/filepath"
	"syscall"
)

type AufsUnmounter struct{}

func (AufsUnmounter) Unmount(dir string) error {
	if err := syscall.Unmount(dir, 0); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(dir, ".."))
}
