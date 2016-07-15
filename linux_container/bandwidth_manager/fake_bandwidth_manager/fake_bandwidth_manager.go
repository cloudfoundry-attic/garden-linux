package fake_bandwidth_manager

import (
	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/lager"
)

type FakeBandwidthManager struct {
	SetLimitsError error
	EnforcedLimits []garden.BandwidthLimits

	GetLimitsError  error
	GetLimitsResult garden.ContainerBandwidthStat
}

func New() *FakeBandwidthManager {
	return &FakeBandwidthManager{}
}

func (m *FakeBandwidthManager) SetLimits(logger lager.Logger, limits garden.BandwidthLimits) error {
	if m.SetLimitsError != nil {
		return m.SetLimitsError
	}

	m.EnforcedLimits = append(m.EnforcedLimits, limits)

	return nil
}

func (m *FakeBandwidthManager) GetLimits(logger lager.Logger) (garden.ContainerBandwidthStat, error) {
	if m.GetLimitsError != nil {
		return garden.ContainerBandwidthStat{}, m.GetLimitsError
	}

	return m.GetLimitsResult, nil
}
