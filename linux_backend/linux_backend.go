package linux_backend

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/sysinfo"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter . Container
type Container interface {
	ID() string
	HasProperties(garden.Properties) bool
	GraceTime() time.Duration

	Start() error

	Snapshot(io.Writer) error
	ResourceSpec() LinuxContainerSpec
	Restore(LinuxContainerSpec) error
	Cleanup() error

	garden.Container
}

//go:generate counterfeiter . ResourcePool
type ResourcePool interface {
	Setup() error
	Acquire(garden.ContainerSpec) (LinuxContainerSpec, error)
	Restore(io.Reader) (LinuxContainerSpec, error)
	Release(LinuxContainerSpec) error
	Prune(keep map[string]bool) error
	MaxContainers() int
}

//go:generate counterfeiter . ContainerProvider
type ContainerProvider interface {
	ProvideContainer(LinuxContainerSpec) Container
}

type ContainerRepository interface {
	All() []Container
	Add(Container)
	FindByHandle(string) (Container, error)
	Query(filter func(Container) bool, logger lager.Logger) []Container
	Delete(Container)
}

//go:generate counterfeiter . HealthChecker
type HealthChecker interface {
	HealthCheck() error
}

type LinuxBackend struct {
	logger lager.Logger

	resourcePool ResourcePool
	systemInfo   sysinfo.Provider
	healthCheck  HealthChecker

	snapshotsPath string
	maxContainers int

	containerRepo     ContainerRepository
	containerProvider ContainerProvider
}

type HandleExistsError struct {
	Handle string
}

func (e HandleExistsError) Error() string {
	return fmt.Sprintf("handle already exists: %s", e.Handle)
}

type FailedToSnapshotError struct {
	OriginalError error
}

func (e FailedToSnapshotError) Error() string {
	return fmt.Sprintf("failed to save snapshot: %s", e.OriginalError)
}

type MaxContainersReachedError struct {
	MaxContainers int
}

func (e MaxContainersReachedError) Error() string {
	return fmt.Sprintf("cannot create more than %d containers", e.MaxContainers)
}

func New(
	logger lager.Logger,
	resourcePool ResourcePool,
	containerRepo ContainerRepository,
	containerProvider ContainerProvider,
	systemInfo sysinfo.Provider,
	healthCheck HealthChecker,
	snapshotsPath string,
	maxContainers int,
) *LinuxBackend {
	return &LinuxBackend{
		logger: logger.Session("backend"),

		resourcePool:  resourcePool,
		systemInfo:    systemInfo,
		healthCheck:   healthCheck,
		snapshotsPath: snapshotsPath,
		maxContainers: maxContainers,

		containerRepo:     containerRepo,
		containerProvider: containerProvider,
	}
}

func (b *LinuxBackend) Setup() error {
	return b.resourcePool.Setup()
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

	containers := b.containerRepo.All()

	for _, container := range containers {
		keep[container.ID()] = true
	}

	if err := mountSysFs(); err != nil {
		return err
	}

	return b.resourcePool.Prune(keep)
}

func (b *LinuxBackend) Ping() error {
	if err := b.healthCheck.HealthCheck(); err != nil {
		return garden.UnrecoverableError{err.Error()}
	}

	return nil
}

func (b *LinuxBackend) Capacity() (garden.Capacity, error) {
	totalMemory, err := b.systemInfo.TotalMemory()
	if err != nil {
		return garden.Capacity{}, err
	}

	totalDisk, err := b.systemInfo.TotalDisk()
	if err != nil {
		return garden.Capacity{}, err
	}

	maxContainers := b.resourcePool.MaxContainers()
	if b.maxContainers > 0 && maxContainers > b.maxContainers {
		maxContainers = b.maxContainers
	}

	return garden.Capacity{
		MemoryInBytes: totalMemory,
		DiskInBytes:   totalDisk,
		MaxContainers: uint64(maxContainers),
	}, nil
}

func (b *LinuxBackend) Create(spec garden.ContainerSpec) (garden.Container, error) {
	if _, err := b.containerRepo.FindByHandle(spec.Handle); spec.Handle != "" && err == nil {
		return nil, HandleExistsError{Handle: spec.Handle}
	}

	if b.maxContainers > 0 {
		containers := b.containerRepo.All()
		if len(containers) >= b.maxContainers {
			return nil, MaxContainersReachedError{
				MaxContainers: b.maxContainers,
			}
		}
	}

	containerSpec, err := b.resourcePool.Acquire(spec)
	if err != nil {
		return nil, err
	}

	container := b.containerProvider.ProvideContainer(containerSpec)

	if err := container.Start(); err != nil {
		b.resourcePool.Release(containerSpec)
		return nil, err
	}

	if err := b.ApplyLimits(container, spec.Limits); err != nil {
		b.resourcePool.Release(containerSpec)
		return nil, err
	}

	b.containerRepo.Add(container)

	return container, nil
}

func (b *LinuxBackend) ApplyLimits(container garden.Container, limits garden.Limits) error {
	if limits.CPU != (garden.CPULimits{}) {
		if err := container.LimitCPU(limits.CPU); err != nil {
			return err
		}
	}

	if limits.Disk != (garden.DiskLimits{}) {
		if err := container.LimitDisk(limits.Disk); err != nil {
			return err
		}
	}

	if limits.Bandwidth != (garden.BandwidthLimits{}) {
		if err := container.LimitBandwidth(limits.Bandwidth); err != nil {
			return err
		}
	}

	if limits.Memory != (garden.MemoryLimits{}) {
		if err := container.LimitMemory(limits.Memory); err != nil {
			return err
		}
	}

	return nil
}

func (b *LinuxBackend) Destroy(handle string) error {
	container, err := b.containerRepo.FindByHandle(handle)
	if err != nil {
		return err
	}

	err = container.Cleanup()
	if err != nil {
		return err
	}

	err = b.resourcePool.Release(container.ResourceSpec())
	if err != nil {
		return err
	}

	b.containerRepo.Delete(container)

	return nil
}

func (b *LinuxBackend) Containers(props garden.Properties) ([]garden.Container, error) {
	logger := b.logger.Session("containers", lager.Data{"props": props})
	logger.Debug("started")
	containers := toGardenContainers(b.containerRepo.Query(withProperties(props), logger))
	logger.Debug("ending", lager.Data{"handles": handles(containers)})
	return containers, nil
}

func handles(containers []garden.Container) []string {
	handles := []string{}
	for _, container := range containers {
		handles = append(handles, container.Handle())
	}
	return handles
}

func (b *LinuxBackend) Lookup(handle string) (garden.Container, error) {
	return b.containerRepo.FindByHandle(handle)
}

func (b *LinuxBackend) BulkInfo(handles []string) (map[string]garden.ContainerInfoEntry, error) {
	containers := b.containerRepo.Query(withHandles(handles), nil)

	infos := make(map[string]garden.ContainerInfoEntry)
	for _, container := range containers {
		info, err := container.Info()
		if err != nil {
			infos[container.Handle()] = garden.ContainerInfoEntry{
				Err: garden.NewError(err.Error()),
			}
		} else {
			infos[container.Handle()] = garden.ContainerInfoEntry{
				Info: info,
			}
		}
	}

	return infos, nil
}

func (b *LinuxBackend) BulkMetrics(handles []string) (map[string]garden.ContainerMetricsEntry, error) {
	containers := b.containerRepo.Query(withHandles(handles), nil)

	metrics := make(map[string]garden.ContainerMetricsEntry)
	for _, container := range containers {
		metric, err := container.Metrics()
		if err != nil {
			metrics[container.Handle()] = garden.ContainerMetricsEntry{
				Err: garden.NewError(err.Error()),
			}
		} else {
			metrics[container.Handle()] = garden.ContainerMetricsEntry{
				Metrics: metric,
			}
		}
	}

	return metrics, nil
}

func (b *LinuxBackend) GraceTime(container garden.Container) time.Duration {
	return container.(Container).GraceTime()
}

func (b *LinuxBackend) Stop() {
	for _, container := range b.containerRepo.All() {
		container.Cleanup()
		err := b.saveSnapshot(container)
		if err != nil {
			b.logger.Error("failed-to-save-snapshot", err, lager.Data{
				"container": container.ID(),
			})
		}
	}
}

func (b *LinuxBackend) restoreSnapshots() {
	sLog := b.logger.Session("restore")

	entries, err := ioutil.ReadDir(b.snapshotsPath)
	if err != nil {
		b.logger.Error("failed-to-read-snapshots", err, lager.Data{
			"from": b.snapshotsPath,
		})
	}

	for _, entry := range entries {
		snapshot := path.Join(b.snapshotsPath, entry.Name())

		lLog := sLog.Session("load", lager.Data{
			"snapshot": entry.Name(),
		})

		lLog.Debug("loading")

		file, err := os.Open(snapshot)
		if err != nil {
			lLog.Error("failed-to-open", err)
		}

		_, err = b.restore(file)
		if err != nil {
			lLog.Error("failed-to-restore", err)
		}
	}
}

func (b *LinuxBackend) saveSnapshot(container Container) error {
	if b.snapshotsPath == "" {
		return nil
	}

	b.logger.Info("save-snapshot", lager.Data{
		"container": container.ID(),
	})

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

func (b *LinuxBackend) restore(snapshot io.Reader) (garden.Container, error) {
	containerSpec, err := b.resourcePool.Restore(snapshot)
	if err != nil {
		return nil, err
	}

	container := b.containerProvider.ProvideContainer(containerSpec)
	container.Restore(containerSpec)

	b.containerRepo.Add(container)
	return container, nil
}

func withHandles(handles []string) func(Container) bool {
	return func(c Container) bool {
		for _, e := range handles {
			if e == c.Handle() {
				return true
			}
		}
		return false
	}
}

func withProperties(props garden.Properties) func(Container) bool {
	return func(c Container) bool {
		return c.HasProperties(props)
	}
}

func toGardenContainers(cs []Container) []garden.Container {
	var result []garden.Container
	for _, c := range cs {
		result = append(result, c)
	}

	return result
}

func mountSysFs() error {
	// mntpoint, err := os.Stat("/sys")
	// if err != nil {
	// 	return err
	// }

	// parent, err := os.Stat("/")
	// if err != nil {
	// 	return err
	// }

	//mount sysfs if not mounted already
	//if mntpoint.Sys().(*syscall.Stat_t).Dev == parent.Sys().(*syscall.Stat_t).Dev {
	//	err = syscall.Mount("sysfs", "/sys", "sysfs", uintptr(0), "")
	//	if err != nil {
	//		return fmt.Errorf("Mounting sysfs failed: %s", err)
	//	}
	//}

	return nil
}
