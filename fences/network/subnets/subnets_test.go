package subnets_test

import (
	"net"
	"runtime"

	"github.com/cloudfoundry-incubator/garden-linux/fences/network/subnets"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subnet Pool", func() {
	var subnetpool subnets.Subnets
	var defaultSubnetPool *net.IPNet

	JustBeforeEach(func() {
		var err error
		subnetpool, err = subnets.New(defaultSubnetPool)
		Ω(err).ShouldNot(HaveOccurred())
	})

	Describe("Capacity", func() {
		Context("when the dynamic allocation net is empty", func() {
			BeforeEach(func() {
				defaultSubnetPool = subnetPool("10.2.3.0/32")
			})

			It("returns zero", func() {
				Ω(subnetpool.Capacity()).Should(Equal(0))
			})
		})
		Context("when the dynamic allocation net is non-empty", func() {
			BeforeEach(func() {
				defaultSubnetPool = subnetPool("10.2.3.0/29")
			})

			It("returns the correct number of subnets initially and repeatedly", func() {
				Ω(subnetpool.Capacity()).Should(Equal(2))
				Ω(subnetpool.Capacity()).Should(Equal(2))
			})

			It("returns the correct capacity after allocating subnets", func() {
				cap := subnetpool.Capacity()

				_, err := subnetpool.AllocateDynamically()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(subnetpool.Capacity()).Should(Equal(cap))

				_, err = subnetpool.AllocateDynamically()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(subnetpool.Capacity()).Should(Equal(cap))
			})
		})
	})

	Describe("Allocating and Releasing", func() {
		Describe("Static Allocation", func() {
			Context("when the requested IP is within the dynamic allocation range", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/29")
				})

				It("returns an appropriate error", func() {
					_, static := networkParms("10.2.3.4/30")

					err := subnetpool.AllocateStatically(static)
					Ω(err).Should(HaveOccurred())
					Ω(err).Should(Equal(subnets.ErrNotAllowed))
				})
			})

			Context("when the requested network subsumes the dynamic allocation range", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.4/30")
				})

				It("returns an appropriate error", func() {
					_, static := networkParms("10.2.3.0/24")

					err := subnetpool.AllocateStatically(static)
					Ω(err).Should(HaveOccurred())
					Ω(err).Should(Equal(subnets.ErrNotAllowed))
				})
			})

			Context("when the requested subnet is not within the dynamic allocation range", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/29")
				})

				Context("allocating a static /30 subnet", func() {
					It("does not return an error", func() {
						_, static := networkParms("10.9.3.4/30")

						err := subnetpool.AllocateStatically(static)
						Ω(err).ShouldNot(HaveOccurred())
					})
				})

				Context("allocating a static /8 subnet", func() {
					It("does not return an error", func() {
						_, static := networkParms("11.0.0.0/8")

						err := subnetpool.AllocateStatically(static)
						Ω(err).ShouldNot(HaveOccurred())
					})
				})

				Context("after a subnet has been allocated, a subsequent request for the same subnet", func() {
					var (
						static *net.IPNet
					)

					JustBeforeEach(func() {
						var err error
						_, static, err = net.ParseCIDR("10.9.3.4/30")
						Ω(err).ShouldNot(HaveOccurred())

						err = subnetpool.AllocateStatically(static)
						Ω(err).ShouldNot(HaveOccurred())
					})

					It("returns an appropriate error", func() {
						err := subnetpool.AllocateStatically(static)
						Ω(err).Should(HaveOccurred())
						Ω(err).Should(Equal(subnets.ErrAlreadyAllocated))
					})

					Context("but after it is released", func() {
						It("allows allocation again", func() {
							err := subnetpool.Release(static)
							Ω(err).ShouldNot(HaveOccurred())

							err = subnetpool.AllocateStatically(static)
							Ω(err).ShouldNot(HaveOccurred())
						})
					})
				})

				Context("after a subnet has been allocated, a subsequent request for an overlapping subnet", func() {
					var (
						firstSubnetPool   *net.IPNet
						firstContainerIP  net.IP
						secondSubnetPool  *net.IPNet
						secondContainerIP net.IP
					)

					JustBeforeEach(func() {
						var err error
						firstContainerIP, firstSubnetPool = networkParms("10.9.3.4/30")
						Ω(err).ShouldNot(HaveOccurred())

						secondContainerIP, secondSubnetPool = networkParms("10.9.3.0/29")
						Ω(err).ShouldNot(HaveOccurred())

						err = subnetpool.AllocateStatically(firstSubnetPool)
						Ω(err).ShouldNot(HaveOccurred())
					})

					It("returns an appropriate error", func() {
						err := subnetpool.AllocateStatically(secondSubnetPool)
						Ω(err).Should(HaveOccurred())
						Ω(err).Should(Equal(subnets.ErrAlreadyAllocated))
					})

					Context("but after it is released", func() {
						It("allows allocation again", func() {
							err := subnetpool.Release(firstSubnetPool)
							Ω(err).ShouldNot(HaveOccurred())

							err = subnetpool.AllocateStatically(secondSubnetPool)
							Ω(err).ShouldNot(HaveOccurred())
						})
					})
				})

				Context("requesting a specific IP address in a static subnet", func() {
					It("does not return an error", func() {
						_, static := networkParms("10.9.3.6/29")

						err := subnetpool.AllocateStatically(static)
						Ω(err).ShouldNot(HaveOccurred())
					})
				})

			})
		})

		Describe("Dynamic Allocation", func() {
			Context("when the pool does not have sufficient IPs to allocate a subnet", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/31")
				})

				It("the first request returns an error", func() {
					_, err := subnetpool.AllocateDynamically()
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when the pool has sufficient IPs to allocate a single subnet", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/30")
				})

				Context("the first request", func() {
					It("succeeds, and returns a /30 network within the subnet", func() {
						network, err := subnetpool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())

						Ω(network).ShouldNot(BeNil())
						Ω(network.String()).Should(Equal("10.2.3.0/30"))
					})
				})

				Context("subsequent requests", func() {
					It("fail, and return an err", func() {
						_, err := subnetpool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())

						_, err = subnetpool.AllocateDynamically()
						Ω(err).Should(HaveOccurred())
					})
				})

				Context("when an allocated network is released", func() {
					It("a subsequent allocation succeeds, and returns the first network again", func() {
						// first
						allocated, err := subnetpool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())

						// second - will fail (sanity check)
						_, err = subnetpool.AllocateDynamically()
						Ω(err).Should(HaveOccurred())

						// release
						err = subnetpool.Release(allocated)
						Ω(err).ShouldNot(HaveOccurred())

						// third - should work now because of release
						network, err := subnetpool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())

						Ω(network).ShouldNot(BeNil())
						Ω(network.String()).Should(Equal(allocated.String()))
					})
				})

				Context("when a network is released twice", func() {
					It("returns an error", func() {
						// first
						allocated, err := subnetpool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())

						// release
						err = subnetpool.Release(allocated)
						Ω(err).ShouldNot(HaveOccurred())

						// release again
						err = subnetpool.Release(allocated)
						Ω(err).Should(HaveOccurred())
						Ω(err).Should(Equal(subnets.ErrReleasedUnallocatedSubnet))
					})
				})
			})

			Context("when the pool has sufficient IPs to allocate two subnets", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/29")
				})

				Context("the second request", func() {
					It("succeeds", func() {
						_, err := subnetpool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())

						_, err = subnetpool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())
					})

					It("returns the second /30 network within the subnet", func() {
						_, err := subnetpool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())

						network, err := subnetpool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())

						Ω(network).ShouldNot(BeNil())
						Ω(network.String()).Should(Equal("10.2.3.4/30"))
					})
				})

				It("allocates distinct networks concurrently", func() {
					prev := runtime.GOMAXPROCS(2)
					defer runtime.GOMAXPROCS(prev)

					Consistently(func() bool {
						_, network, err := net.ParseCIDR("10.0.0.0/29")
						Ω(err).ShouldNot(HaveOccurred())

						pool, err := subnets.New(network)
						Ω(err).ShouldNot(HaveOccurred())

						out := make(chan *net.IPNet)
						go func(out chan *net.IPNet) {
							defer GinkgoRecover()
							n1, err := pool.AllocateDynamically()
							Ω(err).ShouldNot(HaveOccurred())
							out <- n1
						}(out)

						go func(out chan *net.IPNet) {
							defer GinkgoRecover()
							n1, err := pool.AllocateDynamically()
							Ω(err).ShouldNot(HaveOccurred())
							out <- n1
						}(out)

						a := <-out
						b := <-out
						return a.IP.Equal(b.IP)
					}, "100ms", "2ms").ShouldNot(BeTrue())
				})

				It("correctly handles concurrent release of the same network", func() {
					prev := runtime.GOMAXPROCS(2)
					defer runtime.GOMAXPROCS(prev)

					Consistently(func() bool {
						_, network, err := net.ParseCIDR("10.0.0.0/29")
						Ω(err).ShouldNot(HaveOccurred())

						pool, err := subnets.New(network)
						Ω(err).ShouldNot(HaveOccurred())

						n1, err := pool.AllocateDynamically()
						Ω(err).ShouldNot(HaveOccurred())

						out := make(chan error)
						go func(out chan error) {
							defer GinkgoRecover()
							out <- pool.Release(n1)
						}(out)

						go func(out chan error) {
							defer GinkgoRecover()
							out <- pool.Release(n1)
						}(out)

						a := <-out
						b := <-out
						return (a == nil) != (b == nil)
					}, "200ms", "2ms").Should(BeTrue())
				})

				It("correctly handles concurrent allocation of the same network", func() {
					prev := runtime.GOMAXPROCS(2)
					defer runtime.GOMAXPROCS(prev)

					Consistently(func() bool {
						network := subnetPool("10.0.0.0/29")

						pool, err := subnets.New(network)
						Ω(err).ShouldNot(HaveOccurred())

						_, n1 := networkParms("10.1.0.0/30")

						out := make(chan error)
						go func(out chan error) {
							defer GinkgoRecover()
							out <- pool.AllocateStatically(n1)
						}(out)

						go func(out chan error) {
							defer GinkgoRecover()
							out <- pool.AllocateStatically(n1)
						}(out)

						a := <-out
						b := <-out
						return (a == nil) != (b == nil)
					}, "200ms", "2ms").Should(BeTrue())
				})
			})
		})

		Describe("Recovering", func() {
			BeforeEach(func() {
				defaultSubnetPool = subnetPool("10.2.3.0/29")
			})

			Context("an allocation outside the dynamic allocation net", func() {
				It("recovers the first time", func() {
					_, static := networkParms("10.9.3.4/30")

					err := subnetpool.Recover(static)
					Ω(err).ShouldNot(HaveOccurred())
				})
				It("does not allow recovering twice", func() {
					_, static := networkParms("10.9.3.4/30")

					err := subnetpool.Recover(static)
					Ω(err).ShouldNot(HaveOccurred())

					err = subnetpool.Recover(static)
					Ω(err).Should(HaveOccurred())
				})
				It("does not allow allocating after recovery", func() {
					_, static := networkParms("10.9.3.4/30")

					err := subnetpool.Recover(static)
					Ω(err).ShouldNot(HaveOccurred())

					err = subnetpool.AllocateStatically(static)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("an allocation which subsumes the dynamic allocation range", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.4/30")
				})

				It("returns an appropriate error", func() {
					_, static := networkParms("10.2.3.0/24")

					err := subnetpool.Recover(static)
					Ω(err).Should(HaveOccurred())
					Ω(err).Should(Equal(subnets.ErrNotAllowed))
				})
			})

			Context("an allocation inside the dynamic allocation net", func() {
				It("recovers the first time", func() {
					_, static := networkParms("10.2.3.4/30")

					err := subnetpool.Recover(static)
					Ω(err).ShouldNot(HaveOccurred())
				})
				It("does not allow recovering twice", func() {
					_, static := networkParms("10.2.3.4/30")

					err := subnetpool.Recover(static)
					Ω(err).ShouldNot(HaveOccurred())

					err = subnetpool.Recover(static)
					Ω(err).Should(HaveOccurred())
				})
				It("does not dynamically allocate a recovered network", func() {
					_, static := networkParms("10.2.3.4/30")

					err := subnetpool.Recover(static)
					Ω(err).ShouldNot(HaveOccurred())

					network, err := subnetpool.AllocateDynamically()
					Ω(err).ShouldNot(HaveOccurred())

					Ω(network).ShouldNot(BeNil())
					Ω(network.String()).ShouldNot(Equal("10.2.3.4/30"))

					_, err = subnetpool.AllocateDynamically()
					Ω(err).Should(HaveOccurred())
				})
			})

		})

	})
})

func subnetPool(networkString string) *net.IPNet {
	_, subnetPool := networkParms(networkString)
	return subnetPool
}

func networkParms(networkString string) (net.IP, *net.IPNet) {
	containerIP, subnet, err := net.ParseCIDR(networkString)
	Ω(err).ShouldNot(HaveOccurred())
	if containerIP.Equal(subnet.IP) {
		containerIP = nextIP(containerIP)
	}
	return containerIP, subnet
}

func nextIP(ip net.IP) net.IP {
	next := net.ParseIP(ip.String())
	inc(next)
	return next
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
