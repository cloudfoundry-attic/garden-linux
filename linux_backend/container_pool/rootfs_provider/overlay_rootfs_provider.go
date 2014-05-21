package rootfs_provider

import (
	"net/url"
	"os/exec"
	"path"

	"github.com/cloudfoundry/gunk/command_runner"
)

type overlayRootFSProvider struct {
	binPath       string
	overlaysPath  string
	defaultRootFS string
	runner        command_runner.CommandRunner
}

func NewOverlay(
	binPath string,
	overlaysPath string,
	defaultRootFS string,
	runner command_runner.CommandRunner,
) RootFSProvider {
	return &overlayRootFSProvider{
		binPath:       binPath,
		overlaysPath:  overlaysPath,
		defaultRootFS: defaultRootFS,
		runner:        runner,
	}
}

func (provider *overlayRootFSProvider) ProvideRootFS(id string, rootfs *url.URL) (string, error) {
	rootFSPath := provider.defaultRootFS
	if rootfs.Path != "" {
		rootFSPath = rootfs.Path
	}

	err := provider.runner.Run(&exec.Cmd{
		Path: path.Join(provider.binPath, "overlay.sh"),
		Args: []string{"create", path.Join(provider.overlaysPath, id), rootFSPath},
	})
	if err != nil {
		return "", err
	}

	return path.Join(provider.overlaysPath, id, "rootfs"), nil
}

func (provider *overlayRootFSProvider) CleanupRootFS(id string) error {
	return provider.runner.Run(&exec.Cmd{
		Path: path.Join(provider.binPath, "overlay.sh"),
		Args: []string{"cleanup", path.Join(provider.overlaysPath, id)},
	})
}
