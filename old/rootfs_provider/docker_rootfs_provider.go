package rootfs_provider

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/old/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

type dockerRootFSProvider struct {
	graphDriver   graphdriver.Driver
	volumeCreator VolumeCreator
	repoFetcher   repository_fetcher.RepositoryFetcher
	namespacer    Namespacer
	clock         clock.Clock
	mutex         *sync.Mutex
	graphPath     string

	fallback RootFSProvider
}

var ErrInvalidDockerURL = errors.New("invalid docker url")

//go:generate counterfeiter -o fake_graph_driver/fake_graph_driver.go . GraphDriver
type GraphDriver interface {
	graphdriver.Driver
}

func NewDocker(
	repoFetcher repository_fetcher.RepositoryFetcher,
	graphDriver GraphDriver,
	volumeCreator VolumeCreator,
	namespacer Namespacer,
	graphPath string,
	clock clock.Clock,
) (RootFSProvider, error) {
	return &dockerRootFSProvider{
		repoFetcher:   repoFetcher,
		graphDriver:   graphDriver,
		volumeCreator: volumeCreator,
		namespacer:    namespacer,
		graphPath:     graphPath,
		clock:         clock,
		mutex:         &sync.Mutex{},
	}, nil
}

func (provider *dockerRootFSProvider) ProvideRootFS(logger lager.Logger, id string, url *url.URL, shouldNamespace bool) (string, process.Env, error) {
	if len(url.Path) == 0 {
		return "", nil, ErrInvalidDockerURL
	}

	tag := "latest"
	if len(url.Fragment) > 0 {
		tag = url.Fragment
	}

	imageID, envvars, volumes, err := provider.repoFetcher.Fetch(logger, url, tag)
	if err != nil {
		return "", nil, err
	}

	logger.Info(fmt.Sprintf("!!!!!!!!!!!!!!!!!!!!!!!!! Fetch ImageID %s", imageID))

	if shouldNamespace {
		provider.mutex.Lock()
		imageID, err = provider.namespace(imageID)
		provider.mutex.Unlock()
		if err != nil {
			return "", nil, err
		}

		logger.Info(fmt.Sprintf("!!!!!!!!!!!!!!!!!!!!!!!!! NAMESPACED ImageID %s", imageID))
	}

	logger.Info(fmt.Sprintf("!!!!!!!!!!!!!!!!!!!!!!!!! ImageID Before Create %s", imageID))
	err = provider.graphDriver.Create(id, imageID)

	if err != nil {
		return "", nil, err
	}

	rootPath, err := provider.graphDriver.Get(id, "")
	if err != nil {
		return "", nil, err
	}

	logger.Info(fmt.Sprintf("!!!!!!!!!!!!!!!!!!!!!!!!! RootPath %s for ID %s", rootPath, id))

	for _, v := range volumes {
		if err = provider.volumeCreator.Create(rootPath, v); err != nil {
			return "", nil, err
		}
	}

	return rootPath, envvars, nil
}

func (provider *dockerRootFSProvider) namespace(imageID string) (string, error) {
	namespacedImageID := imageID + "@namespaced"
	if !provider.graphDriver.Exists(namespacedImageID) {
		if err := provider.createNamespacedLayer(namespacedImageID, imageID); err != nil {
			return "", err
		}
	}

	return namespacedImageID, nil
}

func (provider *dockerRootFSProvider) createNamespacedLayer(id string, parentId string) error {
	if err := provider.graphDriver.Create(id, parentId); err != nil {
		return err
	}

	namespacedRootfsPath, err := provider.graphDriver.Get(id, parentId)
	if err != nil {
		return err
	}

	if err := provider.namespacer.Namespace(namespacedRootfsPath); err != nil {
		return err
	}

	archive, err := provider.graphDriver.Diff(id, parentId)
	if err != nil {
		return err
	}

	_, err = provider.graphDriver.ApplyDiff(id, parentId, archive)
	if err != nil {
		return err
	}

	return nil
}

func (provider *dockerRootFSProvider) CleanupRootFS(logger lager.Logger, id string) error {
	provider.graphDriver.Put(id)

	if err := syscall.Unmount(filepath.Join(provider.graphPath, "overlayfs", id, "merged"), syscall.MNT_DETACH); err != nil {
		return err
	}

	var err error
	for i := 1; i <= 10; i++ {
		err = provider.graphDriver.Remove(id)
		if err == nil {
			return nil
		}
		logger.Error("cleanup-rootfs", err)
		time.Sleep(time.Millisecond * 500)
	}

	return err
}
