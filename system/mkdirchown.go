package system

import (
	"os"
	"path/filepath"
)

func MkdirChown(path string, uid, gid uint32, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil // The base case of our recursion: this directory already exists!
	}

	if err := MkdirChown(filepath.Dir(path), uid, gid, mode); err != nil {
		return err
	}

	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}

	if err := os.Chown(path, int(uid), int(gid)); err != nil {
		return err
	}

	return nil
}

type MkdirChowner struct{}

func (MkdirChowner) MkdirChown(path string, uid, gid uint32, mode os.FileMode) error {
	return MkdirChown(path, uid, gid, mode)
}
