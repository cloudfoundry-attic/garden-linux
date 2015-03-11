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
			var (
				existingIfc *net.Interface
			)
			BeforeEach(func() {
				var err error
				existingIfc, err = b.Create(name, ip, subnet)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("returns the interface for it", func() {
				ifc, err := b.Create(name, ip, subnet)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(ifc).Should(Equal(existingIfc))
			})

			It("does not change the existing bridge", func() {
				ip2, subnet2, _ := net.ParseCIDR("10.8.0.2/30")
				_, err := b.Create(name, ip2, subnet2)
				Ω(err).ShouldNot(HaveOccurred())

				intf, err := net.InterfaceByName(name)
				Ω(err).ShouldNot(HaveOccurred())

				addrs, err := intf.Addrs()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(addrs[0].String()).Should(Equal(addr))
			})
		})
	})

	Describe("Delete", func() {
		Context("when the bridge exists", func() {
			It("deletes it", func() {
				br, err := b.Create(name, ip, subnet)
				Ω(err).ShouldNot(HaveOccurred())

				// sanity check
				Ω(interfaceNames()).Should(ContainElement(name))

				// delete
				Ω(b.Delete(br.Name)).Should(Succeed())

				// should be gone
				Eventually(interfaceNames).ShouldNot(ContainElement(name))
			})
		})

		Context("when the bridge does not exists", func() {
			It("it returns an error", func() {
				Ω(b.Delete("something")).ShouldNot(Succeed())
			})
		})
	})
})

func interfaceNames() []string {
	intfs, err := net.Interfaces()
	Ω(err).ShouldNot(HaveOccurred())

	v := make([]string, 0)
	for _, i := range intfs {
		v = append(v, i.Name)
	}

	return v
}
