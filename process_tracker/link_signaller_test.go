package process_tracker_test

import (
	"errors"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/process_tracker"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker/fake_linker"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Link Signaller", func() {
	var fakeLink *fake_linker.FakeLinker
	var request *process_tracker.SignalRequest
	var signaller *process_tracker.LinkSignaller

	BeforeEach(func() {
		fakeLink = new(fake_linker.FakeLinker)
		request = &process_tracker.SignalRequest{
			Pid:    12345,
			Link:   fakeLink,
			Signal: syscall.SIGKILL,
		}

		signaller = &process_tracker.LinkSignaller{
			Logger: lagertest.NewTestLogger("test"),
		}
	})

	It("signals process successfully", func() {
		Expect(signaller.Signal(request)).To(Succeed())
		Expect(fakeLink.SendSignalCallCount()).To(Equal(1))
		Expect(fakeLink.SendSignalArgsForCall(0)).To(Equal(request.Signal))
	})

	Context("when the link fails to send the signal", func() {
		var err error
		BeforeEach(func() {
			err = errors.New("what!!")
			fakeLink.SendSignalReturns(err)
		})

		It("returns the error", func() {
			Expect(signaller.Signal(request)).To(MatchError(err))
		})
	})
})
