package rootfs_provider

import (
	"bufio"
	"bytes"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"strings"
	"time"

	"os"

	"syscall"

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

func (provider *overlayRootFSProvider) ProvideRootFS(logger lager.Logger, id string, rootfs *url.URL) (string, process.Env, error) {
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

	var stderr bytes.Buffer
	createOverlay.Stderr = &stderr

	err := pRunner.Run(createOverlay)
	if err != nil {
		return "", nil, fmt.Errorf("overlay.sh: %v, %v", err, strings.TrimRight(stderr.String(), "\n"))
	}

	return path.Join(provider.overlaysPath, id, "rootfs"), nil, nil
}

func (provider *overlayRootFSProvider) CleanupRootFS(logger lager.Logger, id string) error {
	file, err := os.Open("/proc/mounts")

	if err != nil {
		return err
	}

	defer file.Close()

	reader := bufio.NewReader(file)
	scanner := bufio.NewScanner(reader)

	mountPoints := []string{}

	for scanner.Scan() {
		current := scanner.Text()
		if current == provider.overlaysPath {
			parts := strings.Split(current, " ")
			mountPoints = append(mountPoints, parts[1])
		}
	}

	//retry loop up to 4 minutes
	for i := 0; i < 480; i++ {
		if len(mountPoints) == 0 {
			break
		}

		for i, point := range mountPoints {
			fmt.Printf("trying to unmout %s \n", point)

			err := syscall.Unmount(point, syscall.MNT_DETACH)
			if err != nil {
				fmt.Printf("error while unmounting %s %+v \n", point, err)
				break
			}
			mountPoints = append(mountPoints[:i], mountPoints[i+1:]...)
		}
		time.Sleep(time.Millisecond * 500)
	}

	return os.RemoveAll(provider.overlaysPath)
}
