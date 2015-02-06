package subnets_test

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/network/subnets/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("BridgedSubnets", func() {
	var (
		bs                 subnets.BridgedSubnets
		fakeSubnets        *fakes.FakeSubnets
		fakeBing           *fakes.FakeBridgeNameGenerator
		fakeSubnetSelector *fakes.FakeSubnetSelector
		fakeIPSelector     *fakes.FakeIPSelector
	)

	BeforeEach(func() {
		fakeSubnets = &fakes.FakeSubnets{}
		fakeBing = &fakes.FakeBridgeNameGenerator{}
		bs = subnets.NewBridgedSubnetsWithDelegates(fakeSubnets, fakeBing)
	})

	Describe("Allocate", func() {
		BeforeEach(func() {
			fakeSubnetSelector = &fakes.FakeSubnetSelector{}
			fakeIPSelector = &fakes.FakeIPSelector{}
		})

		Context("from a subnet with no prior allocations", func() {
			It("returns the subnet and container IP address allocated by Subnets and generates a new bridge name", func() {
				snip, snipn, _ := net.ParseCIDR("1.2.3.4/24")
				fakeSubnets.AllocateReturns(snipn, snip, true, nil)

				fakeBing.GenerateReturns("bridgeName")

				ipn, ip, bridgeName, err := bs.Allocate(fakeSubnetSelector, fakeIPSelector)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeSubnets.AllocateCallCount()).Should(Equal(1))
				sns, ips := fakeSubnets.AllocateArgsForCall(0)
				Ω(sns).Should(Equal(fakeSubnetSelector))
				Ω(ips).Should(Equal(fakeIPSelector))

				Ω(fakeBing.GenerateCallCount()).Should(Equal(1))
				Ω(bridgeName).Should(Equal("bridgeName"))

				Ω(ipn).Should(Equal(snipn))
				Ω(ip).Should(Equal(snip))
			})
		})

		Context("from a subnet with prior allocations", func() {
			var (
				snipn       *net.IPNet
				snip, snip2 net.IP
				allocs      int
			)

			BeforeEach(func() {
				snip, snipn, _ = net.ParseCIDR("1.2.3.4/24")
				snip2 = net.ParseIP("1.2.3.5")
				allocs = 0
				fakeSubnets.AllocateStub = func(sns subnets.SubnetSelector, ips subnets.IPSelector) (*net.IPNet, net.IP, bool, error) {
					Ω(sns).Should(Equal(fakeSubnetSelector))
					Ω(ips).Should(Equal(fakeIPSelector))

					allocs++
					if allocs == 1 {
						return snipn, snip, true, nil
					} else {
						return snipn, snip2, false, nil
					}
				}
				fakeBing.GenerateReturns("bridgeName")
				bs.Allocate(fakeSubnetSelector, fakeIPSelector)
			})

			It("should return subnet and container IP address allocated by Subnets and the previously generated bridge name", func() {
				ipn, ip, bridgeName, err := bs.Allocate(fakeSubnetSelector, fakeIPSelector)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeSubnets.AllocateCallCount()).Should(Equal(2))

				Ω(fakeBing.GenerateCallCount()).Should(Equal(1))

				Ω(ipn).Should(Equal(snipn))
				Ω(ip).Should(Equal(snip2))
				Ω(bridgeName).Should(Equal("bridgeName"))
			})
		})

		It("when Subnets returns an error on Allocate, this error is returned", func() {
			testError := errors.New("ran out")
			fakeSubnets.AllocateReturns(nil, nil, false, testError)

			_, _, _, err := bs.Allocate(nil, nil)
			Ω(err).Should(Equal(testError))
		})

		Context("when the first allocation is unrecognised", func() {
			It("should panic", func() {
				snip, snipn, _ := net.ParseCIDR("1.2.3.4/24")
				fakeSubnets.AllocateReturns(snipn, snip, false, nil)

				Ω(func() { bs.Allocate(nil, nil) }).Should(Panic())
			})
		})
	})

	Describe("Release", func() {
		var (
			snipn *net.IPNet
			snip  net.IP
		)

		BeforeEach(func() {
			snip, snipn, _ = net.ParseCIDR("1.2.3.4/24")
			fakeBing.GenerateReturns("bridgeName")
		})

		Context("when releasing the last allocation", func() {
			BeforeEach(func() {
				fakeSubnets.AllocateReturns(snipn, snip, true, nil)
				bs.Allocate(nil, nil)
			})

			It("should release the subnet and container IP address using Subnets Release", func() {
				fakeSubnets.ReleaseReturns(true, nil)

				last, bridgeName, err := bs.Release(snipn, snip)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(last).Should(BeTrue())
				Ω(bridgeName).Should(Equal("bridgeName"))
			})

			It("should cause a subsequent allocate to generate a new bridge name", func() {
				fakeSubnets.ReleaseReturns(true, nil)
				bs.Release(snipn, snip)

				bs.Allocate(nil, nil)
				Ω(fakeBing.GenerateCallCount()).Should(Equal(2))
			})

			It("should retain the bridge name across a release which is not the last in the subnet", func() {
				fakeSubnets.AllocateReturns(snipn, snip, false, nil)
				bs.Allocate(nil, nil)

				fakeSubnets.ReleaseReturns(false, nil)

				last, bridgeName, err := bs.Release(snipn, snip)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(last).Should(BeFalse())
				Ω(bridgeName).Should(Equal("bridgeName"))

				fakeSubnets.AllocateReturns(snipn, snip, false, nil)
				fakeBing.GenerateReturns("bridgeName2")
				_, _, bridgeName, err = bs.Allocate(nil, nil)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(bridgeName).Should(Equal("bridgeName"))
			})
		})

		It("when Subnets returns an error on Release, this error is returned", func() {
			testError := errors.New("too bad")
			fakeSubnets.ReleaseReturns(false, testError)

			_, _, err := bs.Release(snipn, snip)
			Ω(err).Should(Equal(testError))
		})

		Context("when releasing an allocation which never was", func() {
			It("should panic", func() {
				fakeSubnets.ReleaseReturns(true, nil)

				Ω(func() { bs.Release(snipn, snip) }).Should(Panic())
			})
		})

		It("panics when passed a nil *net.IPNet", func() {
			Ω(func() (panicValue interface{}) {
				defer func() { panicValue = recover() }()
				bs.Release(nil, net.ParseIP("1.2.3.4"))
				return
			}()).Should(Equal("*net.IPNet parameter must not be nil"))
		})
	})

	Describe("Recover", func() {
		It("it delegates Recover to Subnets and also recovers the bridge name", func() {
			ip, ipn, _ := net.ParseCIDR("1.2.3.4/24")

			fakeSubnets.RecoverReturns(nil)

			Ω(bs.Recover(ipn, ip, "oldBridge")).Should(Succeed())
			Ω(fakeSubnets.RecoverCallCount()).Should(Equal(1))
			ripn, rip := fakeSubnets.RecoverArgsForCall(0)
			Ω(ripn).Should(Equal(ipn))
			Ω(rip).Should(Equal(ip))

			fakeSubnets.ReleaseReturns(true, nil)

			_, bridgeName, err := bs.Release(ipn, ip)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(bridgeName).Should(Equal("oldBridge"))
		})

		It("panics when passed a nil *net.IPNet", func() {
			Ω(func() (panicValue interface{}) {
				defer func() { panicValue = recover() }()
				bs.Recover(nil, net.ParseIP("1.2.3.4"), "oldBridge")
				return
			}()).Should(Equal("*net.IPNet parameter must not be nil"))
		})

		It("when Subnets returns an error on Recover, this error is returned", func() {
			ip, ipn, _ := net.ParseCIDR("1.2.3.4/24")
			testError := errors.New("too bad")
			fakeSubnets.RecoverReturns(testError)

			err := bs.Recover(ipn, ip, "oldBridge")
			Ω(err).Should(Equal(testError))
		})
	})

	Describe("Capacity", func() {
		It("it delegates Capacity to Subnets and returns the result", func() {
			fakeSubnets.CapacityReturns(180)

			Ω(bs.Capacity()).Should(Equal(180))
			Ω(fakeSubnets.CapacityCallCount()).Should(Equal(1))
		})
	})
})
