package devices_test

import (
	"fmt"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/network/devices"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Veth Pair Creation", func() {
	var (
		v                       devices.VethCreator
		hostName, containerName string
	)

	f := func(i *net.Interface, _ error) *net.Interface {
		return i
	}

	l := func(_, _ interface{}, e error) error {
		return e
	}

	BeforeEach(func() {
		hostName = fmt.Sprintf("doesntexist-h-%d", GinkgoParallelNode())
		containerName = fmt.Sprintf("doesntexist-c-%d", GinkgoParallelNode())
	})

	AfterEach(func() {
		Ω(cleanup(hostName)).Should(Succeed())
		Ω(cleanup(containerName)).Should(Succeed())
	})

	Context("when neither host already exists", func() {
		It("creates both interfaces in the host", func() {
			Ω(l(v.Create(hostName, containerName))).Should(Succeed())
			Ω(net.InterfaceByName(hostName)).ShouldNot(BeNil())
			Ω(net.InterfaceByName(containerName)).ShouldNot(BeNil())
		})

		It("returns the created interfaces", func() {
			a, b, err := v.Create(hostName, containerName)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(a).Should(Equal(f(net.InterfaceByName(hostName))))
			Ω(b).Should(Equal(f(net.InterfaceByName(containerName))))
		})
	})

	Context("when one of the interfaces already exists", func() {
		It("returns an error", func() {
			Ω(l(v.Create(hostName, containerName))).Should(Succeed())
			Ω(l(v.Create(hostName, containerName))).ShouldNot(Succeed())
		})
	})
})
