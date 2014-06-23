package network_pool_test

import (
	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/network"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/network_pool"
)

var _ = Describe("Network Pool", func() {
	var pool *network_pool.RealNetworkPool

	BeforeEach(func() {
		_, ipNet, err := net.ParseCIDR("10.254.0.0/22")
		Ω(err).ShouldNot(HaveOccurred())

		pool = network_pool.New(ipNet)
	})

	Describe("acquiring", func() {
		It("takes the next network in the pool", func() {
			network1, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(network1.String()).Should(Equal("10.254.0.0/30"))

			network2, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(network2.String()).Should(Equal("10.254.0.4/30"))
		})

		Context("when the pool is exhausted", func() {
			It("returns an error", func() {
				for i := 0; i < 256; i++ {
					_, err := pool.Acquire()
					Ω(err).ShouldNot(HaveOccurred())
				}

				_, err := pool.Acquire()
				Ω(err).Should(HaveOccurred())
			})
		})
	})

	Describe("removing", func() {
		It("acquires a specific network from the pool", func() {
			_, ipNet, err := net.ParseCIDR("10.254.0.0/30")
			Ω(err).ShouldNot(HaveOccurred())

			err = pool.Remove(network.New(ipNet))
			Ω(err).ShouldNot(HaveOccurred())

			for i := 0; i < (256 - 1); i++ {
				network, err := pool.Acquire()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(network.String()).ShouldNot(Equal("10.254.0.0/30"))
			}

			_, err = pool.Acquire()
			Ω(err).Should(HaveOccurred())
		})

		Context("when the resource is already acquired", func() {
			It("returns a PortTakenError", func() {
				network, err := pool.Acquire()
				Ω(err).ShouldNot(HaveOccurred())

				err = pool.Remove(network)
				Ω(err).Should(Equal(network_pool.NetworkTakenError{network}))
			})
		})
	})

	Describe("releasing", func() {
		It("places a network back and the end of the pool", func() {
			first, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())

			pool.Release(first)

			for i := 0; i < 255; i++ {
				_, err := pool.Acquire()
				Ω(err).ShouldNot(HaveOccurred())
			}

			last, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(last).Should(Equal(first))
		})

		Context("when the released network is out of the range", func() {
			It("does not add it to the pool", func() {
				_, smallIPNet, err := net.ParseCIDR("10.255.0.0/32")
				Ω(err).ShouldNot(HaveOccurred())

				kiddiePool := network_pool.New(smallIPNet)

				_, err = kiddiePool.Acquire()
				Ω(err).ShouldNot(HaveOccurred())

				_, err = kiddiePool.Acquire()
				Ω(err).Should(HaveOccurred())

				outOfRangeNetwork, err := pool.Acquire()
				Ω(err).ShouldNot(HaveOccurred())

				kiddiePool.Release(outOfRangeNetwork)

				_, err = kiddiePool.Acquire()
				Ω(err).Should(HaveOccurred())
			})
		})
	})

	Describe("InitialSize", func() {
		It("returns the count of maximum available networks", func() {
			Ω(pool.InitialSize()).Should(Equal(256))
			_, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(pool.InitialSize()).Should(Equal(256))
		})
	})

	Describe("getting the network", func() {
		It("returns the network's *net.IPNet", func() {
			Ω(pool.Network().String()).Should(Equal("10.254.0.0/22"))
		})
	})
})
