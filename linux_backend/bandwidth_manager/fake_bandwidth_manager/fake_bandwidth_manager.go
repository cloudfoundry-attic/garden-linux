package fake_bandwidth_manager

import (
	"github.com/cloudfoundry-incubator/garden/warden"
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

func (m *FakeBandwidthManager) SetLimits(limits warden.BandwidthLimits) error {
	if m.SetLimitsError != nil {
		return m.SetLimitsError
	}

	m.EnforcedLimits = append(m.EnforcedLimits, limits)

	return nil
}

func (m *FakeBandwidthManager) GetLimits() (warden.ContainerBandwidthStat, error) {
	if m.GetLimitsError != nil {
		return warden.ContainerBandwidthStat{}, m.GetLimitsError
	}

	return m.GetLimitsResult, nil
}
