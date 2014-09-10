package rootfs_provider

import (
	"net/url"
	"os/exec"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/logging"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
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

func (provider *overlayRootFSProvider) ProvideRootFS(logger lager.Logger, id string, rootfs *url.URL) (string, []string, error) {
	rootFSPath := provider.defaultRootFS
	if rootfs.Path != "" {
		rootFSPath = rootfs.Path
	}

	pRunner := logging.Runner{
		CommandRunner: provider.runner,
		Logger:        logger,
	}

	createOverlay := exec.Command(
		path.Join(provider.binPath, "overlay.sh"),
		"create", path.Join(provider.overlaysPath, id), rootFSPath,
	)

	err := pRunner.Run(createOverlay)
	if err != nil {
		return "", nil, err
	}

	return path.Join(provider.overlaysPath, id, "rootfs"), nil, nil
}

func (provider *overlayRootFSProvider) CleanupRootFS(logger lager.Logger, id string) error {
	pRunner := logging.Runner{
		CommandRunner: provider.runner,
		Logger:        logger,
	}

	destroyOverlay := exec.Command(
		path.Join(provider.binPath, "overlay.sh"),
		"cleanup", path.Join(provider.overlaysPath, id),
	)

	return pRunner.Run(destroyOverlay)
}
