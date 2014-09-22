package fake_bandwidth_manager

import (
	"github.com/cloudfoundry-incubator/garden/api"
	"github.com/pivotal-golang/lager"
)

type FakeBandwidthManager struct {
	SetLimitsError error
	EnforcedLimits []api.BandwidthLimits

	GetLimitsError  error
	GetLimitsResult api.ContainerBandwidthStat
}

func New() *FakeBandwidthManager {
	return &FakeBandwidthManager{}
}

func (m *FakeBandwidthManager) SetLimits(logger lager.Logger, limits api.BandwidthLimits) error {
	if m.SetLimitsError != nil {
		return m.SetLimitsError
	}

	m.EnforcedLimits = append(m.EnforcedLimits, limits)

	return nil
}

func (m *FakeBandwidthManager) GetLimits(logger lager.Logger) (api.ContainerBandwidthStat, error) {
	if m.GetLimitsError != nil {
		return api.ContainerBandwidthStat{}, m.GetLimitsError
	}

	return m.GetLimitsResult, nil
}
