// This file was generated by counterfeiter
package fake_msg_sender

import (
	"sync"

	"code.cloudfoundry.org/garden-linux/process_tracker"
)

type FakeMsgSender struct {
	SendMsgStub        func(msg []byte) error
	sendMsgMutex       sync.RWMutex
	sendMsgArgsForCall []struct {
		msg []byte
	}
	sendMsgReturns struct {
		result1 error
	}
}

func (fake *FakeMsgSender) SendMsg(msg []byte) error {
	fake.sendMsgMutex.Lock()
	fake.sendMsgArgsForCall = append(fake.sendMsgArgsForCall, struct {
		msg []byte
	}{msg})
	fake.sendMsgMutex.Unlock()
	if fake.SendMsgStub != nil {
		return fake.SendMsgStub(msg)
	} else {
		return fake.sendMsgReturns.result1
	}
}

func (fake *FakeMsgSender) SendMsgCallCount() int {
	fake.sendMsgMutex.RLock()
	defer fake.sendMsgMutex.RUnlock()
	return len(fake.sendMsgArgsForCall)
}

func (fake *FakeMsgSender) SendMsgArgsForCall(i int) []byte {
	fake.sendMsgMutex.RLock()
	defer fake.sendMsgMutex.RUnlock()
	return fake.sendMsgArgsForCall[i].msg
}

func (fake *FakeMsgSender) SendMsgReturns(result1 error) {
	fake.SendMsgStub = nil
	fake.sendMsgReturns = struct {
		result1 error
	}{result1}
}

var _ process_tracker.MsgSender = new(FakeMsgSender)
