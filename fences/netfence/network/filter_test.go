package network_test

import (
	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/iptables/fakes"
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

		ItMutatesIPTables := func(network string, port uint32, portRange string, protocol garden.Protocol, icmpType, icmpCode int32) {
			It("should mutate IP tables", func() {
				err := filter.NetOut(network, port, portRange, protocol, icmpType, icmpCode)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeChain.PrependFilterRuleCallCount()).Should(Equal(1))
				destProtocol, dest, destPort, destPortRange, destIcmpType, destIcmpCode := fakeChain.PrependFilterRuleArgsForCall(0)
				Ω(destProtocol).Should(Equal(protocol))
				Ω(dest).Should(Equal(network))
				Ω(destPort).Should(Equal(port))
				Ω(destPortRange).Should(Equal(portRange))
				Ω(destIcmpType).Should(Equal(icmpType))
				Ω(destIcmpCode).Should(Equal(icmpCode))
			})
		}

		ItAllowsPortOrPortRange := func(protocol garden.Protocol) {
			Context("and no network is specified", func() {
				Context("and neither port nor port range are specified", func() {
					It("should return an error", func() {
						err := filter.NetOut("", 0, "", protocol, -1, -1)
						Ω(err).Should(MatchError("invalid rule: either network or port (range) must be specified"))
					})
				})

				Context("and a port is specified", func() {
					ItMutatesIPTables("", 80, "", protocol, -1, -1)
				})

				Context("and a port range is specified", func() {
					ItMutatesIPTables("", 0, "8080:8081", protocol, -1, -1)
				})
			})

			Context("and a network is specified", func() {
				Context("and a port range is specified", func() {
					Context("and no port is specified", func() {
						ItMutatesIPTables("1.2.3.4/24", 0, "8080:8081", protocol, -1, -1)
					})

					Context("and a port is specified", func() {
						It("should return an error", func() {
							err := filter.NetOut("1.2.3.4/24", 78, "8080:8081", protocol, -1, -1)
							Ω(err).Should(MatchError("invalid rule: port and port range cannot both be specified"))
						})
					})
				})

				Context("and a port specified", func() {
					Context("and no port range is specified", func() {
						ItMutatesIPTables("1.2.3.4/24", 70, "", protocol, -1, -1)
					})

					Context("and a port range is specified", func() {
						It("should return an error", func() {
							err := filter.NetOut("1.2.3.4/24", 78, "8080:8081", protocol, -1, -1)
							Ω(err).Should(MatchError("invalid rule: port and port range cannot both be specified"))
						})
					})
				})
			})
		}

		ItDoesNotAllowPortOrPortRange := func(protocol garden.Protocol) {
			Context("when no port or port range are specified", func() {
				ItMutatesIPTables("1.2.3.4/24", 0, "", protocol, -1, -1)
			})

			Context("when port is specified", func() {
				It("should return an error", func() {
					err := filter.NetOut("1.2.3.4/24", 78, "", protocol, -1, -1)
					Ω(err).Should(MatchError("invalid rule: a port (range) can only be specified with protocol TCP or UDP"))
				})
			})

			Context("when port range is specified", func() {
				It("should return an error", func() {
					err := filter.NetOut("1.2.3.4/24", 0, "80:81", protocol, -1, -1)
					Ω(err).Should(MatchError("invalid rule: a port (range) can only be specified with protocol TCP or UDP"))
				})
			})
		}

		ItDoesNotAllowIcmpCodeOrType := func(protocol garden.Protocol) {
			Context("and an ICMP type is specified", func() {
				It("should return an error", func() {
					err := filter.NetOut("", 80, "", protocol, -1, 8)
					Ω(err).Should(MatchError("invalid rule: icmp code or icmp type can only be specified with protocol ICMP"))
				})
			})

			Context("and an ICMP code is specified", func() {
				It("should return an error", func() {
					err := filter.NetOut("", 80, "", protocol, 8, -1)
					Ω(err).Should(MatchError("invalid rule: icmp code or icmp type can only be specified with protocol ICMP"))
				})
			})
		}

		Context("when the protocol is TCP", func() {
			ItAllowsPortOrPortRange(garden.ProtocolTCP)
			ItDoesNotAllowIcmpCodeOrType(garden.ProtocolTCP)
		})

		Context("when the protocol is UDP", func() {
			ItAllowsPortOrPortRange(garden.ProtocolUDP)
			ItDoesNotAllowIcmpCodeOrType(garden.ProtocolUDP)
		})

		Context("when the protocol is ALL", func() {
			ItDoesNotAllowPortOrPortRange(garden.ProtocolAll)
			ItDoesNotAllowIcmpCodeOrType(garden.ProtocolAll)
		})

		Context("when the protocol is ICMP", func() {
			ItDoesNotAllowPortOrPortRange(garden.ProtocolICMP)

			Context("and icmp type is specified", func() {
				ItMutatesIPTables("1.2.3.4/24", 0, "", garden.ProtocolICMP, 7, -1)
			})

			Context("and icmp code is specified", func() {
				ItMutatesIPTables("1.2.3.4/24", 0, "", garden.ProtocolICMP, -1, 8)
			})
		})
	})
})
