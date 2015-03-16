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
		Ω(filter).ShouldNot(BeNil())
	})

	Context("Setup", func() {
		It("sets up the chain", func() {
			Ω(filter.Setup("logPrefix")).Should(Succeed())
			Ω(fakeChain.SetupCallCount()).Should(Equal(1))
			Ω(fakeChain.SetupArgsForCall(0)).Should(Equal("logPrefix"))
		})

		Context("when chain setup returns an error", func() {
			It("Setup wraps the error and returns it", func() {
				fakeChain.SetupReturns(errors.New("x"))
				err := filter.Setup("logPrefix")
				Ω(err).Should(MatchError("network: log chain setup: x"))
			})
		})
	})

	Context("TearDown", func() {
		It("tears down the chain", func() {
			filter.TearDown()
			Ω(fakeChain.TearDownCallCount()).Should(Equal(1))
		})
	})

	Context("NetOut", func() {
		It("should mutate IP tables", func() {
			rule := garden.NetOutRule{}
			err := filter.NetOut(rule)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeChain.PrependFilterRuleCallCount()).Should(Equal(1))
			passedRule := fakeChain.PrependFilterRuleArgsForCall(0)
			Ω(passedRule).Should(Equal(passedRule))
		})

		It("returns an error if one occurs", func() {
			fakeChain.PrependFilterRuleReturns(errors.New("iptables says no"))
			Ω(filter.NetOut(garden.NetOutRule{})).Should(MatchError("iptables says no"))
		})
	})
})
