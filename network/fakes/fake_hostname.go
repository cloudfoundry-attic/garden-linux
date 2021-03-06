// This file was generated by counterfeiter
package fakes

import (
	"sync"

	"code.cloudfoundry.org/garden-linux/network"
)

type FakeHostname struct {
	SetHostnameStub        func(hostName string) error
	setHostnameMutex       sync.RWMutex
	setHostnameArgsForCall []struct {
		hostName string
	}
	setHostnameReturns struct {
		result1 error
	}
}

func (fake *FakeHostname) SetHostname(hostName string) error {
	fake.setHostnameMutex.Lock()
	fake.setHostnameArgsForCall = append(fake.setHostnameArgsForCall, struct {
		hostName string
	}{hostName})
	fake.setHostnameMutex.Unlock()
	if fake.SetHostnameStub != nil {
		return fake.SetHostnameStub(hostName)
	} else {
		return fake.setHostnameReturns.result1
	}
}

func (fake *FakeHostname) SetHostnameCallCount() int {
	fake.setHostnameMutex.RLock()
	defer fake.setHostnameMutex.RUnlock()
	return len(fake.setHostnameArgsForCall)
}

func (fake *FakeHostname) SetHostnameArgsForCall(i int) string {
	fake.setHostnameMutex.RLock()
	defer fake.setHostnameMutex.RUnlock()
	return fake.setHostnameArgsForCall[i].hostName
}

func (fake *FakeHostname) SetHostnameReturns(result1 error) {
	fake.SetHostnameStub = nil
	fake.setHostnameReturns = struct {
		result1 error
	}{result1}
}

var _ network.Hostname = new(FakeHostname)
