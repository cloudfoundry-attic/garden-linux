package fake_quota_manager

import (
	"sync"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/pivotal-golang/lager"
)

type FakeQuotaManager struct {
	SetLimitsError error
	GetLimitsError error
	GetUsageError  error

	GetLimitsResult warden.DiskLimits
	GetUsageResult  warden.ContainerDiskStat

	MountPointResult string

	Limited map[uint32]warden.DiskLimits

	enabled bool

	sync.RWMutex
}

func New() *FakeQuotaManager {
	return &FakeQuotaManager{
		Limited: make(map[uint32]warden.DiskLimits),

		enabled: true,
	}
}

func (m *FakeQuotaManager) SetLimits(logger lager.Logger, uid uint32, limits warden.DiskLimits) error {
	if m.SetLimitsError != nil {
		return m.SetLimitsError
	}

	m.Lock()
	defer m.Unlock()

	m.Limited[uid] = limits

	return nil
}

func (m *FakeQuotaManager) GetLimits(logger lager.Logger, uid uint32) (warden.DiskLimits, error) {
	if m.GetLimitsError != nil {
		return warden.DiskLimits{}, m.GetLimitsError
	}

	m.RLock()
	defer m.RUnlock()

	return m.GetLimitsResult, nil
}

func (m *FakeQuotaManager) GetUsage(logger lager.Logger, uid uint32) (warden.ContainerDiskStat, error) {
	if m.GetUsageError != nil {
		return warden.ContainerDiskStat{}, m.GetUsageError
	}

	m.RLock()
	defer m.RUnlock()

	return m.GetUsageResult, nil
}

func (m *FakeQuotaManager) MountPoint() string {
	return m.MountPointResult
}

func (m *FakeQuotaManager) Disable() {
	m.enabled = false
}

func (m *FakeQuotaManager) IsEnabled() bool {
	return m.enabled
}
