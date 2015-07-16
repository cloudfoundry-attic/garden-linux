package containerizer_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_capabilities"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CapabilitiesStep", func() {
	var capabilities *fake_capabilities.FakeCapabilities
	var step *containerizer.CapabilitiesStep
	var drop bool

	JustBeforeEach(func() {
		capabilities = new(fake_capabilities.FakeCapabilities)
		step = &containerizer.CapabilitiesStep{
			Drop:         drop,
			Capabilities: capabilities,
		}
	})

	Context("when drop is false", func() {
		BeforeEach(func() {
			drop = false
		})

		It("does not limit capabilities", func() {
			Expect(step.Run()).To(Succeed())
			Expect(capabilities.LimitCallCount()).To(Equal(0))
		})
	})

	Context("when drop is true", func() {
		BeforeEach(func() {
			drop = true
		})

		It("limits capabilities", func() {
			Expect(step.Run()).To(Succeed())
			Expect(capabilities.LimitCallCount()).To(Equal(1))
		})

		It("uses the default whitelist", func() {
			Expect(step.Run()).To(Succeed())
			Expect(capabilities.LimitArgsForCall(0)).To(Equal(false))
		})

		Context("when limit fails", func() {
			JustBeforeEach(func() {
				capabilities.LimitReturns(errors.New("banana"))
			})

			It("returns an error", func() {
				err := step.Run()
				Expect(err).To(MatchError(ContainSubstring("banana")))
			})
		})
	})
})
