package devices_test

import (
	"fmt"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/network/devices"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bridge Management", func() {
	var (
		b      devices.Bridge
		name   string
		addr   string
		ip     net.IP
		subnet *net.IPNet
	)

	BeforeEach(func() {
		name = fmt.Sprintf("gdn-test-intf-%d", GinkgoParallelNode())

		var err error
		addr = "10.9.0.1/30"
		ip, subnet, err = net.ParseCIDR(addr)
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		cleanup(name)
	})

	Describe("Create", func() {
		Context("when the bridge does not already exist", func() {
			It("creates a bridge", func() {
				_, err := b.Create(name, ip, subnet)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("sets the bridge name", func() {
				bridge, err := b.Create(name, ip, subnet)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(bridge.Name).Should(Equal(name))
			})

			It("sets the bridge address", func() {
				bridge, err := b.Create(name, ip, subnet)
				Ω(err).ShouldNot(HaveOccurred())

				addrs, err := bridge.Addrs()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(addrs).Should(HaveLen(1))
				Ω(addrs[0].String()).Should(Equal(addr))
			})
		})

		Context("when the bridge exists", func() {
			BeforeEach(func() {
				_, err := b.Create(name, ip, subnet)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("returns an error", func() {
				_, err := b.Create(name, ip, subnet)
				Ω(err).Should(HaveOccurred())
			})

			It("does not change the existing bridge", func() {
				ip2, subnet2, _ := net.ParseCIDR("10.8.0.2/30")
				_, err := b.Create(name, ip2, subnet2)
				Ω(err).Should(HaveOccurred())

				intf, err := net.InterfaceByName(name)
				Ω(err).ShouldNot(HaveOccurred())

				addrs, err := intf.Addrs()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(addrs[0].String()).Should(Equal(addr))
			})
		})
	})
})
