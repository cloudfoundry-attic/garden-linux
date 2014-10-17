package net_fence_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/net_fence"

	"errors"
	"flag"
	"github.com/cloudfoundry-incubator/garden-linux/net_fence/subnets"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net"
)

var _ = Describe("Network Fence Flags", func() {

	Describe("The networkPool flag", func() {

		var (
			flagset            *flag.FlagSet
			newedWithIpn       *net.IPNet
			cmdline            []string
			defaultNetworkPool *net.IPNet
			returnedSubnets    subnets.Subnets
		)

		JustBeforeEach(func() {
			var err error
			_, defaultNetworkPool, err = net.ParseCIDR(net_fence.DefaultNetworkPool)
			Ω(err).ShouldNot(HaveOccurred())

			returnedSubnets, err = subnets.New(defaultNetworkPool)
			Ω(err).ShouldNot(HaveOccurred())

			net_fence.NewSubnets = func(ipn *net.IPNet) (subnets.Subnets, error) {
				newedWithIpn = ipn
				return returnedSubnets, nil
			}

			flagset = &flag.FlagSet{}
			net_fence.InitializeFlags(flagset)

			flagset.Parse(cmdline)
		})

		Context("when not supplied", func() {
			BeforeEach(func() {
				cmdline = []string{}
			})

			It("configures the subnet pool with the default value", func() {
				subnets, err := net_fence.Initialize()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(newedWithIpn).Should(Equal(defaultNetworkPool))
				Ω(subnets).Should(Equal(returnedSubnets))
			})

			It("returns the error if creating the subnet fails", func() {
				errOnNew := errors.New("o no")
				net_fence.NewSubnets = func(ipn *net.IPNet) (subnets.Subnets, error) {
					return nil, errOnNew
				}

				_, err := net_fence.Initialize()
				Ω(err).Should(Equal(errOnNew))
			})
		})

		Context("when supplied", func() {
			Context("and when it's valid", func() {
				BeforeEach(func() {
					cmdline = []string{"-networkPool=1.2.3.4/5"}
				})

				It("configures the network pool with the given value", func() {
					subnets, err := net_fence.Initialize()
					Ω(err).ShouldNot(HaveOccurred())

					_, network, err := net.ParseCIDR("1.2.3.4/5")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(newedWithIpn).Should(Equal(network))
					Ω(subnets).Should(Equal(returnedSubnets))
				})
			})

			Context("and when it's not valid", func() {
				BeforeEach(func() {
					cmdline = []string{`-networkPool="1.2.3.4/5"`} // flags cannot contain quotes
				})

				It("returns an error", func() {
					_, err := net_fence.Initialize()
					Ω(err).Should(HaveOccurred())
				})

				It("names the invalid parameter in the error message", func() {
					_, err := net_fence.Initialize()
					Ω(err).Should(HaveOccurred())
					Ω(err.Error()).Should(ContainSubstring("networkPool"))
				})
			})
		})

	})

})
