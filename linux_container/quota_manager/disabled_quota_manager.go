package quota_manager

import (
	"github.com/cloudfoundry-incubator/garden"
	"github.com/pivotal-golang/lager"
)

type DisabledQuotaManager struct{}

func (DisabledQuotaManager) SetLimits(logger lager.Logger, containerRootFSPath string, limits garden.DiskLimits) error {
	return nil
}

func (DisabledQuotaManager) GetLimits(logger lager.Logger, containerRootFSPath string) (garden.DiskLimits, error) {
	return garden.DiskLimits{}, nil
}

func (DisabledQuotaManager) GetUsage(logger lager.Logger, containerRootFSPath string) (garden.ContainerDiskStat, error) {
	return garden.ContainerDiskStat{}, nil
}

func (DisabledQuotaManager) Setup() error {
	return nil
}

func (DisabledQuotaManager) IsEnabled() bool {
	return false
}
