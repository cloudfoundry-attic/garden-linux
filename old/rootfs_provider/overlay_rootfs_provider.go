package rootfs_provider

import (
	"bytes"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/old/logging"
	"github.com/cloudfoundry-incubator/garden-linux/process"
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

func (provider *overlayRootFSProvider) ProvideRootFS(logger lager.Logger, id string, rootfs *url.URL, namespace bool) (string, process.Env, error) {
	rootFSPath := provider.defaultRootFS
	if rootfs.Path != "" {
		rootFSPath = rootfs.Path
	}

	// Rootfs path in container spec is empty
	if rootFSPath == "" {
		return "", nil, fmt.Errorf("RootFSPath: is a required parameter, since no default rootfs was provided to the server. To provide a default rootfs, use the --rootfs flag on startup.")
	}

	pRunner := logging.Runner{
		CommandRunner: provider.runner,
		Logger:        logger,
	}

	createOverlay := exec.Command(
		path.Join(provider.binPath, "overlay.sh"),
		"create", path.Join(provider.overlaysPath, id), rootFSPath,
	)

	var stderr bytes.Buffer
	createOverlay.Stderr = &stderr

	err := pRunner.Run(createOverlay)
	if err != nil {
		return "", nil, fmt.Errorf("overlay.sh: %v, %v", err, strings.TrimRight(stderr.String(), "\n"))
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
