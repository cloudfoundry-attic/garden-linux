package linux_backend

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sync"
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/warden-linux/system_info"
)

type Container interface {
	ID() string
	Properties() warden.Properties
	GraceTime() time.Duration

	Start() error

	Snapshot(io.Writer) error
	Cleanup()

	warden.Container
}

type ContainerPool interface {
	Setup() error
	Create(warden.ContainerSpec) (Container, error)
	Restore(io.Reader) (Container, error)
	Destroy(Container) error
	Prune(keep map[string]bool) error
	MaxContainers() int
}

type LinuxBackend struct {
	containerPool ContainerPool
	systemInfo    system_info.Provider
	snapshotsPath string

	containers      map[string]Container
	containersMutex *sync.RWMutex
}

type UnknownHandleError struct {
	Handle string
}

func (e UnknownHandleError) Error() string {
	return "unknown handle: " + e.Handle
}

type FailedToSnapshotError struct {
	OriginalError error
}

func (e FailedToSnapshotError) Error() string {
	return fmt.Sprintf("failed to save snapshot: %s", e.OriginalError)
}

func New(containerPool ContainerPool, systemInfo system_info.Provider, snapshotsPath string) *LinuxBackend {
	return &LinuxBackend{
		containerPool: containerPool,
		systemInfo:    systemInfo,
		snapshotsPath: snapshotsPath,

		containers:      make(map[string]Container),
		containersMutex: new(sync.RWMutex),
	}
}

func (b *LinuxBackend) Setup() error {
	return b.containerPool.Setup()
}

func (b *LinuxBackend) Start() error {
	if b.snapshotsPath != "" {
		_, err := os.Stat(b.snapshotsPath)
		if err == nil {
			b.restoreSnapshots()
			os.RemoveAll(b.snapshotsPath)
		}

		err = os.MkdirAll(b.snapshotsPath, 0755)
		if err != nil {
			return err
		}
	}

	keep := map[string]bool{}

	b.containersMutex.RLock()
	containers := b.containers
	b.containersMutex.RUnlock()

	for _, container := range containers {
		keep[container.ID()] = true
	}

	return b.containerPool.Prune(keep)
}

func (b *LinuxBackend) Capacity() (warden.Capacity, error) {
	totalMemory, err := b.systemInfo.TotalMemory()
	if err != nil {
		return warden.Capacity{}, err
	}

	totalDisk, err := b.systemInfo.TotalDisk()
	if err != nil {
		return warden.Capacity{}, err
	}

	return warden.Capacity{
		MemoryInBytes: totalMemory,
		DiskInBytes:   totalDisk,
		MaxContainers: uint64(b.containerPool.MaxContainers()),
	}, nil
}

func (b *LinuxBackend) Create(spec warden.ContainerSpec) (warden.Container, error) {
	container, err := b.containerPool.Create(spec)
	if err != nil {
		return nil, err
	}

	err = container.Start()
	if err != nil {
		return nil, err
	}

	b.containersMutex.Lock()
	b.containers[container.Handle()] = container
	b.containersMutex.Unlock()

	return container, nil
}

func (b *LinuxBackend) Destroy(handle string) error {
	b.containersMutex.RLock()
	container, found := b.containers[handle]
	b.containersMutex.RUnlock()

	if !found {
		return UnknownHandleError{handle}
	}

	err := b.containerPool.Destroy(container)
	if err != nil {
		return err
	}

	b.containersMutex.Lock()
	delete(b.containers, container.Handle())
	b.containersMutex.Unlock()

	return nil
}

func (b *LinuxBackend) Containers(filter warden.Properties) (containers []warden.Container, err error) {
	b.containersMutex.RLock()
	defer b.containersMutex.RUnlock()

	for _, container := range b.containers {
		if containerHasProperties(container, filter) {
			containers = append(containers, container)
		}
	}

	return containers, nil
}

func (b *LinuxBackend) Lookup(handle string) (warden.Container, error) {
	b.containersMutex.RLock()
	defer b.containersMutex.RUnlock()

	container, found := b.containers[handle]
	if !found {
		return nil, UnknownHandleError{handle}
	}

	return container, nil
}

func (b *LinuxBackend) GraceTime(container warden.Container) time.Duration {
	return container.(Container).GraceTime()
}

func (b *LinuxBackend) Stop() {
	b.containersMutex.RLock()
	defer b.containersMutex.RUnlock()

	for _, container := range b.containers {
		container.Cleanup()
		err := b.saveSnapshot(container)
		if err != nil {
			log.Println(err)
		}
	}
}

func (b *LinuxBackend) restoreSnapshots() {
	entries, err := ioutil.ReadDir(b.snapshotsPath)
	if err != nil {
		log.Println("failed to read snapshots", b.snapshotsPath, err)
	}

	for _, entry := range entries {
		snapshot := path.Join(b.snapshotsPath, entry.Name())

		log.Println("loading snapshot for", entry.Name())

		file, err := os.Open(snapshot)
		if err != nil {
			log.Println("failed to open", entry.Name(), err)
		}

		_, err = b.restore(file)
		if err != nil {
			log.Println("failed to restore", entry.Name(), err)
		}
	}
}

func (b *LinuxBackend) saveSnapshot(container Container) error {
	if b.snapshotsPath == "" {
		return nil
	}

	log.Println("saving snapshot for", container.ID())

	snapshotPath := path.Join(b.snapshotsPath, container.ID())

	snapshot, err := os.Create(snapshotPath)
	if err != nil {
		return &FailedToSnapshotError{err}
	}

	err = container.Snapshot(snapshot)
	if err != nil {
		return &FailedToSnapshotError{err}
	}

	return snapshot.Close()
}

func (b *LinuxBackend) restore(snapshot io.Reader) (warden.Container, error) {
	container, err := b.containerPool.Restore(snapshot)
	if err != nil {
		return nil, err
	}

	b.containersMutex.Lock()
	b.containers[container.Handle()] = container
	b.containersMutex.Unlock()

	return container, nil
}

func containerHasProperties(container Container, properties warden.Properties) bool {
	containerProps := container.Properties()

	for key, val := range properties {
		cval, ok := containerProps[key]
		if !ok {
			return false
		}

		if cval != val {
			return false
		}
	}

	return true
}
