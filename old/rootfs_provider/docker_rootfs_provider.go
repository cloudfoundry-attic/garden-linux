package rootfs_provider

import (
	"net/url"
	"os"
	"os/exec"
	"sync"
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
	copier        Copier
	clock         clock.Clock
	mutex         *sync.Mutex

	fallback RootFSProvider
}

//go:generate counterfeiter -o fake_graph_driver/fake_graph_driver.go . GraphDriver
type GraphDriver interface {
	graphdriver.Driver
}

//go:generate counterfeiter -o fake_copier/fake_copier.go . Copier
type Copier interface {
	Copy(src, dest string) error
}

func NewDocker(
	repoFetcher repository_fetcher.RepositoryFetcher,
	graphDriver GraphDriver,
	volumeCreator VolumeCreator,
	namespacer Namespacer,
	copier Copier,
	clock clock.Clock,
) (RootFSProvider, error) {
	return &dockerRootFSProvider{
		repoFetcher:   repoFetcher,
		graphDriver:   graphDriver,
		volumeCreator: volumeCreator,
		namespacer:    namespacer,
		copier:        copier,
		clock:         clock,
		mutex:         &sync.Mutex{},
	}, nil
}

func (provider *dockerRootFSProvider) ProvideRootFS(logger lager.Logger, id string, url *url.URL, shouldNamespace bool) (string, process.Env, error) {
	tag := "latest"
	if len(url.Fragment) > 0 {
		tag = url.Fragment
	}

	imageID, envvars, volumes, err := provider.repoFetcher.Fetch(logger, url, tag)
	if err != nil {
		return "", nil, err
	}

	if shouldNamespace {
		provider.mutex.Lock()
		imageID, err = provider.namespace(imageID)
		provider.mutex.Unlock()
		if err != nil {
			return "", nil, err
		}
	}

	err = provider.graphDriver.Create(id, imageID)
	if err != nil {
		return "", nil, err
	}

	rootPath, err := provider.graphDriver.Get(id, "")
	if err != nil {
		return "", nil, err
	}

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
	var err error
	var path string
	if path, err = provider.createAufsWorkaroundLayer(id, parentId); err != nil {
		return err
	}

	return provider.namespacer.Namespace(path)
}

// aufs directory permissions dont overlay cleanly, so we create an empty layer
// and copy the parent layer in while namespacing (rather than just creating a
// regular overlay layer and doing the namespacing directly inside it)
func (provider *dockerRootFSProvider) createAufsWorkaroundLayer(id, parentId string) (string, error) {
	errs := func(err error) (string, error) {
		return "", err
	}

	originalRootfs, err := provider.graphDriver.Get(parentId, "")
	if err != nil {
		return errs(err)
	}

	if err := provider.graphDriver.Create(id, ""); err != nil { // empty layer
		return errs(err)
	}

	namespacedRootfs, err := provider.graphDriver.Get(id, "") // path where empty layer is
	if err != nil {
		return errs(err)
	}

	if err := provider.copier.Copy(originalRootfs, namespacedRootfs); err != nil {
		return errs(err)
	}

	return namespacedRootfs, nil
}

func (provider *dockerRootFSProvider) CleanupRootFS(logger lager.Logger, id string) error {
	provider.graphDriver.Put(id)

	var err error
	maxAttempts := 10

	for errorCount := 0; errorCount < maxAttempts; errorCount++ {
		err = provider.graphDriver.Remove(id)
		if err == nil {
			break
		}

		logger.Error("cleanup-rootfs", err, lager.Data{
			"current-attempts": errorCount + 1,
			"max-attempts":     maxAttempts,
		})

		provider.clock.Sleep(200 * time.Millisecond)
	}

	return err
}

type ShellOutCp struct {
	WorkDir string
}

func (s ShellOutCp) Copy(src, dest string) error {
	if err := os.Remove(dest); err != nil {
		return err
	}

	return exec.Command("cp", "-a", src, dest).Run()
}
