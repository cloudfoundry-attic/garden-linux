package port_pool_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/port_pool"
)

var _ = Describe("Port pool", func() {
	Describe("acquiring", func() {
		It("returns the next available port from the pool", func() {
			pool := port_pool.New(10000, 5)

			port1, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())

			port2, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(port1).Should(Equal(uint32(10000)))
			Ω(port2).Should(Equal(uint32(10001)))
		})

		Context("when the pool is exhausted", func() {
			It("returns an error", func() {
				pool := port_pool.New(10000, 5)

				for i := 0; i < 5; i++ {
					_, err := pool.Acquire()
					Ω(err).ShouldNot(HaveOccurred())
				}

				_, err := pool.Acquire()
				Ω(err).Should(HaveOccurred())
			})
		})
	})

	Describe("removing", func() {
		It("acquires a specific port from the pool", func() {
			pool := port_pool.New(10000, 2)

			err := pool.Remove(10000)
			Ω(err).ShouldNot(HaveOccurred())

			port, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(port).Should(Equal(uint32(10001)))

			_, err = pool.Acquire()
			Ω(err).Should(HaveOccurred())
		})

		Context("when the resource is already acquired", func() {
			It("returns a PortTakenError", func() {
				pool := port_pool.New(10000, 2)

				port, err := pool.Acquire()
				Ω(err).ShouldNot(HaveOccurred())

				err = pool.Remove(port)
				Ω(err).Should(Equal(port_pool.PortTakenError{port}))
			})
		})
	})

	Describe("releasing", func() {
		It("places a port back at the end of the pool", func() {
			pool := port_pool.New(10000, 2)

			port1, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(port1).Should(Equal(uint32(10000)))

			pool.Release(port1)

			port2, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(port2).Should(Equal(uint32(10001)))

			nextPort, err := pool.Acquire()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(nextPort).Should(Equal(uint32(10000)))
		})

		Context("when the released port is out of the range", func() {
			It("does not add it to the pool", func() {
				pool := port_pool.New(10000, 0)

				pool.Release(20000)

				_, err := pool.Acquire()
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when the released port is already released", func() {
			It("does not duplicate it", func() {
				pool := port_pool.New(10000, 2)

				port1, err := pool.Acquire()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(port1).Should(Equal(uint32(10000)))

				pool.Release(port1)
				pool.Release(port1)

				port2, err := pool.Acquire()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(port2).ShouldNot(Equal(port1))

				port3, err := pool.Acquire()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(port3).Should(Equal(port1))

				_, err = pool.Acquire()
				Ω(err).Should(HaveOccurred())
			})
		})
	})
})
