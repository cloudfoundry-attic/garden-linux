package process_tracker_test

import (
	"encoding/json"
	"errors"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/iodaemon/link"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker/fake_msg_sender"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Link Signaller", func() {
	var msgSender *fake_msg_sender.FakeMsgSender
	var request *process_tracker.SignalRequest
	var signaller *process_tracker.LinkSignaller
	var signalSent syscall.Signal

	BeforeEach(func() {
		signalSent = syscall.SIGTERM
	})

	JustBeforeEach(func() {
		msgSender = new(fake_msg_sender.FakeMsgSender)
		request = &process_tracker.SignalRequest{
			Pid:    12345,
			Link:   msgSender,
			Signal: signalSent,
		}

		signaller = &process_tracker.LinkSignaller{}
	})

	It("signals process successfully", func() {
		Expect(signaller.Signal(request)).To(Succeed())
		Expect(msgSender.SendMsgCallCount()).To(Equal(1))

		data, err := json.Marshal(&link.SignalMsg{Signal: signalSent})
		Expect(err).ToNot(HaveOccurred())
		Expect(msgSender.SendMsgArgsForCall(0)).To(Equal(data))
	})

	Context("when the link fails to send the signal", func() {
		var err error
		JustBeforeEach(func() {
			err = errors.New("what!!")
			msgSender.SendMsgReturns(err)
		})

		It("returns the error", func() {
			Expect(signaller.Signal(request)).To(MatchError(err))
		})
	})
})
