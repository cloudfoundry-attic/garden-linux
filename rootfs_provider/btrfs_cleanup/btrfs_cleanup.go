package btrfs_cleanup

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
)

type BtrfsRootFSRemover struct {
	Runner          command_runner.CommandRunner
	GraphDriver     rootfs_provider.GraphDriver
	BtrfsMountPoint string
	RemoveAll       func(dir string) error
}

func (c *BtrfsRootFSRemover) CleanupRootFS(logger lager.Logger, id string) error {
	c.GraphDriver.Put(id)

	if err := c.clean(id); err != nil {
		return err
	}

	return c.GraphDriver.Remove(id)
}

func (c *BtrfsRootFSRemover) clean(id string) error {
	layerPath, err := c.GraphDriver.Get(id, "")
	if err != nil {
		return err
	}

	listSubvolumesOutput, err := c.run(exec.Command("btrfs", "subvolume", "list", c.BtrfsMountPoint))
	if err != nil {
		return err
	}

	subvols := finalColumns(strings.Split(listSubvolumesOutput, "\n"))
	sort.Sort(deepestFirst(subvols))

	for _, subvolume := range subvols {
		subvolumeAbsPath := filepath.Join(c.BtrfsMountPoint, subvolume)

		if strings.Contains(subvolumeAbsPath, layerPath) && subvolumeAbsPath != layerPath {
			c.RemoveAll(subvolumeAbsPath)
			if _, err := c.run(exec.Command("btrfs", "subvolume", "delete", subvolumeAbsPath)); err != nil {
				return err
			}
		}
	}

	return nil
}

func finalColumns(lines []string) []string {
	result := make([]string, 0)
	for _, line := range lines {
		cols := strings.Fields(line)
		if len(cols) == 0 {
			continue
		}

		result = append(result, cols[len(cols)-1])
	}

	return result
}

func (c *BtrfsRootFSRemover) run(cmd *exec.Cmd) (string, error) {
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	if err := c.Runner.Run(cmd); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

type deepestFirst []string

func (a deepestFirst) Len() int           { return len(a) }
func (a deepestFirst) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a deepestFirst) Less(i, j int) bool { return len(a[i]) > len(a[j]) }
