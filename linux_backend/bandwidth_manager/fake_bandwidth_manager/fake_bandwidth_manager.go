package fake_bandwidth_manager

import (
	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/pivotal-golang/lager"
)

type FakeBandwidthManager struct {
	SetLimitsError error
	EnforcedLimits []warden.BandwidthLimits

	GetLimitsError  error
	GetLimitsResult warden.ContainerBandwidthStat
}

func New() *FakeBandwidthManager {
	return &FakeBandwidthManager{}
}

func (m *FakeBandwidthManager) SetLimits(logger lager.Logger, limits warden.BandwidthLimits) error {
	if m.SetLimitsError != nil {
		return m.SetLimitsError
	}

	m.EnforcedLimits = append(m.EnforcedLimits, limits)

	return nil
}

func (m *FakeBandwidthManager) GetLimits(logger lager.Logger) (warden.ContainerBandwidthStat, error) {
	if m.GetLimitsError != nil {
		return warden.ContainerBandwidthStat{}, m.GetLimitsError
	}

	return m.GetLimitsResult, nil
}
