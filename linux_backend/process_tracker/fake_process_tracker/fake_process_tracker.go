// This file was generated by counterfeiter
package fake_process_tracker

import (
	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/cloudfoundry-incubator/warden-linux/linux_backend/process_tracker"
	"os/exec"
	"sync"
)

type FakeProcessTracker struct {
	RunStub        func(*exec.Cmd) (uint32, chan warden.ProcessStream, error)
	runMutex       sync.RWMutex
	runArgsForCall []struct {
		arg1 *exec.Cmd
	}
	runReturns struct {
		result1 uint32
		result2 chan warden.ProcessStream
		result3 error
	}
	AttachStub        func(uint32) (chan warden.ProcessStream, error)
	attachMutex       sync.RWMutex
	attachArgsForCall []struct {
		arg1 uint32
	}
	attachReturns struct {
		result1 chan warden.ProcessStream
		result2 error
	}
	RestoreStub        func(processID uint32)
	restoreMutex       sync.RWMutex
	restoreArgsForCall []struct {
		arg1 uint32
	}
	ActiveProcessIDsStub        func() []uint32
	activeProcessIDsMutex       sync.RWMutex
	activeProcessIDsArgsForCall []struct{}
	activeProcessIDsReturns     struct {
		result1 []uint32
	}
	UnlinkAllStub        func()
	unlinkAllMutex       sync.RWMutex
	unlinkAllArgsForCall []struct{}
}

func (fake *FakeProcessTracker) Run(arg1 *exec.Cmd) (uint32, chan warden.ProcessStream, error) {
	fake.runMutex.Lock()
	defer fake.runMutex.Unlock()
	fake.runArgsForCall = append(fake.runArgsForCall, struct {
		arg1 *exec.Cmd
	}{arg1})
	if fake.RunStub != nil {
		return fake.RunStub(arg1)
	} else {
		return fake.runReturns.result1, fake.runReturns.result2, fake.runReturns.result3
	}
}

func (fake *FakeProcessTracker) RunCallCount() int {
	fake.runMutex.RLock()
	defer fake.runMutex.RUnlock()
	return len(fake.runArgsForCall)
}

func (fake *FakeProcessTracker) RunArgsForCall(i int) *exec.Cmd {
	fake.runMutex.RLock()
	defer fake.runMutex.RUnlock()
	return fake.runArgsForCall[i].arg1
}

func (fake *FakeProcessTracker) RunReturns(result1 uint32, result2 chan warden.ProcessStream, result3 error) {
	fake.runReturns = struct {
		result1 uint32
		result2 chan warden.ProcessStream
		result3 error
	}{result1, result2, result3}
}

func (fake *FakeProcessTracker) Attach(arg1 uint32) (chan warden.ProcessStream, error) {
	fake.attachMutex.Lock()
	defer fake.attachMutex.Unlock()
	fake.attachArgsForCall = append(fake.attachArgsForCall, struct {
		arg1 uint32
	}{arg1})
	if fake.AttachStub != nil {
		return fake.AttachStub(arg1)
	} else {
		return fake.attachReturns.result1, fake.attachReturns.result2
	}
}

func (fake *FakeProcessTracker) AttachCallCount() int {
	fake.attachMutex.RLock()
	defer fake.attachMutex.RUnlock()
	return len(fake.attachArgsForCall)
}

func (fake *FakeProcessTracker) AttachArgsForCall(i int) uint32 {
	fake.attachMutex.RLock()
	defer fake.attachMutex.RUnlock()
	return fake.attachArgsForCall[i].arg1
}

func (fake *FakeProcessTracker) AttachReturns(result1 chan warden.ProcessStream, result2 error) {
	fake.attachReturns = struct {
		result1 chan warden.ProcessStream
		result2 error
	}{result1, result2}
}

func (fake *FakeProcessTracker) Restore(arg1 uint32) {
	fake.restoreMutex.Lock()
	defer fake.restoreMutex.Unlock()
	fake.restoreArgsForCall = append(fake.restoreArgsForCall, struct {
		arg1 uint32
	}{arg1})
	if fake.RestoreStub != nil {
		fake.RestoreStub(arg1)
	}
}

func (fake *FakeProcessTracker) RestoreCallCount() int {
	fake.restoreMutex.RLock()
	defer fake.restoreMutex.RUnlock()
	return len(fake.restoreArgsForCall)
}

func (fake *FakeProcessTracker) RestoreArgsForCall(i int) uint32 {
	fake.restoreMutex.RLock()
	defer fake.restoreMutex.RUnlock()
	return fake.restoreArgsForCall[i].arg1
}

func (fake *FakeProcessTracker) ActiveProcessIDs() []uint32 {
	fake.activeProcessIDsMutex.Lock()
	defer fake.activeProcessIDsMutex.Unlock()
	fake.activeProcessIDsArgsForCall = append(fake.activeProcessIDsArgsForCall, struct{}{})
	if fake.ActiveProcessIDsStub != nil {
		return fake.ActiveProcessIDsStub()
	} else {
		return fake.activeProcessIDsReturns.result1
	}
}

func (fake *FakeProcessTracker) ActiveProcessIDsCallCount() int {
	fake.activeProcessIDsMutex.RLock()
	defer fake.activeProcessIDsMutex.RUnlock()
	return len(fake.activeProcessIDsArgsForCall)
}

func (fake *FakeProcessTracker) ActiveProcessIDsReturns(result1 []uint32) {
	fake.activeProcessIDsReturns = struct {
		result1 []uint32
	}{result1}
}

func (fake *FakeProcessTracker) UnlinkAll() {
	fake.unlinkAllMutex.Lock()
	defer fake.unlinkAllMutex.Unlock()
	fake.unlinkAllArgsForCall = append(fake.unlinkAllArgsForCall, struct{}{})
	if fake.UnlinkAllStub != nil {
		fake.UnlinkAllStub()
	}
}

func (fake *FakeProcessTracker) UnlinkAllCallCount() int {
	fake.unlinkAllMutex.RLock()
	defer fake.unlinkAllMutex.RUnlock()
	return len(fake.unlinkAllArgsForCall)
}

var _ ProcessTracker = new(FakeProcessTracker)