package network_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/iptables/fakes"
	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filter", func() {
	var (
		fakeChainFactory *fakes.FakeChainFactory
		fakeChain        *fakes.FakeChain
	)

	BeforeEach(func() {
		fakeChainFactory = new(fakes.FakeChainFactory)
		fakeChain = new(fakes.FakeChain)
		fakeChainFactory.CreateChainReturns(fakeChain)
	})

	Describe("FilterFactory", func() {
		var filterFactory network.FilterFactory
		BeforeEach(func() {
			filterFactory = network.NewFilterFactory("tag", fakeChainFactory)
		})

		It("constructs a filter from a container id", func() {
			filter := filterFactory.Create("id")
			Ω(filter).ShouldNot(BeNil())
			Ω(fakeChainFactory.CreateChainCallCount()).Should(Equal(1))
			Ω(fakeChainFactory.CreateChainArgsForCall(0)).Should(Equal("w-tag-instance-id"))
		})
	})

	Context("NetOut", func() {
		var filter network.Filter
		BeforeEach(func() {
			filterFactory := network.NewFilterFactory("tag", fakeChainFactory)
			filter = filterFactory.Create("id")
			Ω(filter).ShouldNot(BeNil())
			Ω(fakeChainFactory.CreateChainCallCount()).Should(Equal(1))
			Ω(fakeChainFactory.CreateChainArgsForCall(0)).Should(Equal("w-tag-instance-id"))
		})

		It("should mutate iptables correctly when port is specified", func() {
			err := filter.NetOut("1.2.3.4/24", 8080, "", api.ProtocolTCP)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(fakeChain.PrependFilterRuleCallCount()).Should(Equal(1))
			protocol, dest, destPort, destPortRange := fakeChain.PrependFilterRuleArgsForCall(0)
			Ω(protocol).Should(Equal(api.ProtocolTCP))
			Ω(dest).Should(Equal("1.2.3.4/24"))
			Ω(destPort).Should(Equal(uint32(8080)))
			Ω(destPortRange).Should(Equal(""))
		})

		It("should mutate iptables correctly when port is specified but no network", func() {
			err := filter.NetOut("", 8080, "", api.ProtocolTCP)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(fakeChain.PrependFilterRuleCallCount()).Should(Equal(1))
			protocol, dest, destPort, destPortRange := fakeChain.PrependFilterRuleArgsForCall(0)
			Ω(protocol).Should(Equal(api.ProtocolTCP))
			Ω(dest).Should(Equal(""))
			Ω(destPort).Should(Equal(uint32(8080)))
			Ω(destPortRange).Should(Equal(""))
		})

		It("should mutate iptables correctly when port range is specified", func() {
			err := filter.NetOut("1.2.3.4/24", 0, "80:81", api.ProtocolTCP)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(fakeChain.PrependFilterRuleCallCount()).Should(Equal(1))
			protocol, dest, destPort, destPortRange := fakeChain.PrependFilterRuleArgsForCall(0)
			Ω(protocol).Should(Equal(api.ProtocolTCP))
			Ω(dest).Should(Equal("1.2.3.4/24"))
			Ω(destPort).Should(Equal(uint32(0)))
			Ω(destPortRange).Should(Equal("80:81"))
		})

		It("should mutate iptables correctly when port range is specified but no network", func() {
			err := filter.NetOut("", 0, "80:81", api.ProtocolTCP)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(fakeChain.PrependFilterRuleCallCount()).Should(Equal(1))
			protocol, dest, destPort, destPortRange := fakeChain.PrependFilterRuleArgsForCall(0)
			Ω(protocol).Should(Equal(api.ProtocolTCP))
			Ω(dest).Should(Equal(""))
			Ω(destPort).Should(Equal(uint32(0)))
			Ω(destPortRange).Should(Equal("80:81"))
		})

		It("return an error if port is specified and protocol is all", func() {
			err := filter.NetOut("1.2.3.4/24", 8080, "", api.ProtocolAll)
			Ω(err).Should(HaveOccurred())
			Ω(err).Should(MatchError("invalid rule: a port (range) can only be specified with protocol TCP"))
		})

		It("return an error if port range is specified and protocol is all", func() {
			err := filter.NetOut("1.2.3.4/24", 0, "80:81", api.ProtocolAll)
			Ω(err).Should(HaveOccurred())
			Ω(err).Should(MatchError("invalid rule: a port (range) can only be specified with protocol TCP"))
		})

		It("return an error if network, port, and port range are omitted", func() {
			err := filter.NetOut("", 0, "", api.ProtocolAll)
			Ω(err).Should(HaveOccurred())
			Ω(err).Should(MatchError("invalid rule: either network or port (range) must be specified"))
		})

		It("return an error if port and port range are specified", func() {
			err := filter.NetOut("", 80, "80:80", api.ProtocolTCP)
			Ω(err).Should(HaveOccurred())
			Ω(err).Should(MatchError("invalid rule: port and port range cannot both be specified"))
		})
	})

})
