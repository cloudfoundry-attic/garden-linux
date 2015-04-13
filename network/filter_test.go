package network_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/iptables/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filter", func() {
	var (
		fakeChain *fakes.FakeChain
		filter    network.Filter
	)

	BeforeEach(func() {
		fakeChain = new(fakes.FakeChain)
		filter = network.NewFilter(fakeChain)
		Expect(filter).ToNot(BeNil())
	})

	Context("Setup", func() {
		It("sets up the chain", func() {
			Expect(filter.Setup("logPrefix")).To(Succeed())
			Expect(fakeChain.SetupCallCount()).To(Equal(1))
			Expect(fakeChain.SetupArgsForCall(0)).To(Equal("logPrefix"))
		})

		Context("when chain setup returns an error", func() {
			It("Setup wraps the error and returns it", func() {
				fakeChain.SetupReturns(errors.New("x"))
				err := filter.Setup("logPrefix")
				Expect(err).To(MatchError("network: log chain setup: x"))
			})
		})
	})

	Context("TearDown", func() {
		It("tears down the chain", func() {
			filter.TearDown()
			Expect(fakeChain.TearDownCallCount()).To(Equal(1))
		})
	})

	Context("NetOut", func() {
		It("should mutate IP tables", func() {
			rule := garden.NetOutRule{}
			err := filter.NetOut(rule)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeChain.PrependFilterRuleCallCount()).To(Equal(1))
			passedRule := fakeChain.PrependFilterRuleArgsForCall(0)
			Expect(passedRule).To(Equal(passedRule))
		})

		It("returns an error if one occurs", func() {
			fakeChain.PrependFilterRuleReturns(errors.New("iptables says no"))
			Expect(filter.NetOut(garden.NetOutRule{})).To(MatchError("iptables says no"))
		})
	})
})
