// This file was generated by counterfeiter
package fake_quota_manager

import (
	"sync"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/pivotal-golang/lager"
)

type FakeQuotaManager struct {
	SetLimitsStub        func(logger lager.Logger, containerRootFSPath string, limits garden.DiskLimits) error
	setLimitsMutex       sync.RWMutex
	setLimitsArgsForCall []struct {
		logger              lager.Logger
		containerRootFSPath string
		limits              garden.DiskLimits
	}
	setLimitsReturns struct {
		result1 error
	}
	GetLimitsStub        func(logger lager.Logger, containerRootFSPath string) (garden.DiskLimits, error)
	getLimitsMutex       sync.RWMutex
	getLimitsArgsForCall []struct {
		logger              lager.Logger
		containerRootFSPath string
	}
	getLimitsReturns struct {
		result1 garden.DiskLimits
		result2 error
	}
	GetUsageStub        func(logger lager.Logger, containerRootFSPath string) (garden.ContainerDiskStat, error)
	getUsageMutex       sync.RWMutex
	getUsageArgsForCall []struct {
		logger              lager.Logger
		containerRootFSPath string
	}
	getUsageReturns struct {
		result1 garden.ContainerDiskStat
		result2 error
	}
	SetupStub        func() error
	setupMutex       sync.RWMutex
	setupArgsForCall []struct{}
	setupReturns     struct {
		result1 error
	}
}

func (fake *FakeQuotaManager) SetLimits(logger lager.Logger, containerRootFSPath string, limits garden.DiskLimits) error {
	fake.setLimitsMutex.Lock()
	fake.setLimitsArgsForCall = append(fake.setLimitsArgsForCall, struct {
		logger              lager.Logger
		containerRootFSPath string
		limits              garden.DiskLimits
	}{logger, containerRootFSPath, limits})
	fake.setLimitsMutex.Unlock()
	if fake.SetLimitsStub != nil {
		return fake.SetLimitsStub(logger, containerRootFSPath, limits)
	} else {
		return fake.setLimitsReturns.result1
	}
}

func (fake *FakeQuotaManager) SetLimitsCallCount() int {
	fake.setLimitsMutex.RLock()
	defer fake.setLimitsMutex.RUnlock()
	return len(fake.setLimitsArgsForCall)
}

func (fake *FakeQuotaManager) SetLimitsArgsForCall(i int) (lager.Logger, string, garden.DiskLimits) {
	fake.setLimitsMutex.RLock()
	defer fake.setLimitsMutex.RUnlock()
	return fake.setLimitsArgsForCall[i].logger, fake.setLimitsArgsForCall[i].containerRootFSPath, fake.setLimitsArgsForCall[i].limits
}

func (fake *FakeQuotaManager) SetLimitsReturns(result1 error) {
	fake.SetLimitsStub = nil
	fake.setLimitsReturns = struct {
		result1 error
	}{result1}
}

func (fake *FakeQuotaManager) GetLimits(logger lager.Logger, containerRootFSPath string) (garden.DiskLimits, error) {
	fake.getLimitsMutex.Lock()
	fake.getLimitsArgsForCall = append(fake.getLimitsArgsForCall, struct {
		logger              lager.Logger
		containerRootFSPath string
	}{logger, containerRootFSPath})
	fake.getLimitsMutex.Unlock()
	if fake.GetLimitsStub != nil {
		return fake.GetLimitsStub(logger, containerRootFSPath)
	} else {
		return fake.getLimitsReturns.result1, fake.getLimitsReturns.result2
	}
}

func (fake *FakeQuotaManager) GetLimitsCallCount() int {
	fake.getLimitsMutex.RLock()
	defer fake.getLimitsMutex.RUnlock()
	return len(fake.getLimitsArgsForCall)
}

func (fake *FakeQuotaManager) GetLimitsArgsForCall(i int) (lager.Logger, string) {
	fake.getLimitsMutex.RLock()
	defer fake.getLimitsMutex.RUnlock()
	return fake.getLimitsArgsForCall[i].logger, fake.getLimitsArgsForCall[i].containerRootFSPath
}

func (fake *FakeQuotaManager) GetLimitsReturns(result1 garden.DiskLimits, result2 error) {
	fake.GetLimitsStub = nil
	fake.getLimitsReturns = struct {
		result1 garden.DiskLimits
		result2 error
	}{result1, result2}
}

func (fake *FakeQuotaManager) GetUsage(logger lager.Logger, containerRootFSPath string) (garden.ContainerDiskStat, error) {
	fake.getUsageMutex.Lock()
	fake.getUsageArgsForCall = append(fake.getUsageArgsForCall, struct {
		logger              lager.Logger
		containerRootFSPath string
	}{logger, containerRootFSPath})
	fake.getUsageMutex.Unlock()
	if fake.GetUsageStub != nil {
		return fake.GetUsageStub(logger, containerRootFSPath)
	} else {
		return fake.getUsageReturns.result1, fake.getUsageReturns.result2
	}
}

func (fake *FakeQuotaManager) GetUsageCallCount() int {
	fake.getUsageMutex.RLock()
	defer fake.getUsageMutex.RUnlock()
	return len(fake.getUsageArgsForCall)
}

func (fake *FakeQuotaManager) GetUsageArgsForCall(i int) (lager.Logger, string) {
	fake.getUsageMutex.RLock()
	defer fake.getUsageMutex.RUnlock()
	return fake.getUsageArgsForCall[i].logger, fake.getUsageArgsForCall[i].containerRootFSPath
}

func (fake *FakeQuotaManager) GetUsageReturns(result1 garden.ContainerDiskStat, result2 error) {
	fake.GetUsageStub = nil
	fake.getUsageReturns = struct {
		result1 garden.ContainerDiskStat
		result2 error
	}{result1, result2}
}

func (fake *FakeQuotaManager) Setup() error {
	fake.setupMutex.Lock()
	fake.setupArgsForCall = append(fake.setupArgsForCall, struct{}{})
	fake.setupMutex.Unlock()
	if fake.SetupStub != nil {
		return fake.SetupStub()
	} else {
		return fake.setupReturns.result1
	}
}

func (fake *FakeQuotaManager) SetupCallCount() int {
	fake.setupMutex.RLock()
	defer fake.setupMutex.RUnlock()
	return len(fake.setupArgsForCall)
}

func (fake *FakeQuotaManager) SetupReturns(result1 error) {
	fake.SetupStub = nil
	fake.setupReturns = struct {
		result1 error
	}{result1}
}

var _ linux_container.QuotaManager = new(FakeQuotaManager)
