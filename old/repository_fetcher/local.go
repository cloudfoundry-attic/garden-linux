package repository_fetcher

import (
	"crypto/md5"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/pivotal-golang/lager"
)

type IDer interface {
	ID(path string) string
}

type Local struct {
	Graph             Graph
	DefaultRootFSPath string
	IDer              IDer

	mu sync.RWMutex
}

func (l *Local) Fetch(
	logger lager.Logger,
	repoURL *url.URL,
	tag string,
) (string, process.Env, []string, error) {

	path := repoURL.Path
	if len(path) == 0 {
		path = l.DefaultRootFSPath
	}

	if len(path) == 0 {
		return "", nil, nil, errors.New("RootFSPath: is a required parameter, since no default rootfs was provided to the server. To provide a default rootfs, use the --rootfs flag on startup.")
	}

	id, err := l.fetch(path)
	return id, nil, nil, err
}

func (l *Local) fetch(path string) (string, error) {
	id := l.IDer.ID(path)

	l.mu.RLock()
	if l.Graph.Exists(id) {
		return id, nil // use cache
	}
	l.mu.RUnlock()

	// synchronize all downloads, we could optimize by only mutexing around each
	// particular rootfs path, but in practice importing local rootfses is decently fast,
	// and concurrently importing local rootfses is rare.
	l.mu.Lock()
	defer l.mu.Unlock()

	tar, err := archive.Tar(path, archive.Uncompressed)
	if err != nil {
		return "", fmt.Errorf("fetch local rootfs: %s", err)
	}

	if err := l.Graph.Register(&image.Image{ID: id}, tar); err != nil {
		return "", fmt.Errorf("fetch local rootfs: %v", err)
	}

	return id, nil
}

type MD5ID struct{}

func (MD5ID) ID(path string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(path)))
}
