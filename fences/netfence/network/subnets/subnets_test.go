package subnets_test

import (
	"net"
	"runtime"

	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/subnets"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subnet Pool", func() {
	var subnetpool subnets.Subnets
	var defaultSubnetPool *net.IPNet

	JustBeforeEach(func() {
		var err error
		subnetpool, err = subnets.NewSubnets(defaultSubnetPool)
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
				defaultSubnetPool = subnetPool("10.2.3.0/27")
			})

			It("returns the correct number of subnets initially and repeatedly", func() {
				Ω(subnetpool.Capacity()).Should(Equal(8))
				Ω(subnetpool.Capacity()).Should(Equal(8))
			})

			It("returns the correct capacity after allocating subnets", func() {
				cap := subnetpool.Capacity()

				_, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(subnetpool.Capacity()).Should(Equal(cap))

				_, _, _, err = subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(subnetpool.Capacity()).Should(Equal(cap))
			})
		})
	})

	Describe("Allocating and Releasing", func() {
		Describe("Static Subnet Allocation", func() {
			Context("when the requested subnet is within the dynamic allocation range", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/29")
				})

				It("returns an appropriate error", func() {
					_, static := networkParms("10.2.3.4/30")

					_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
					Ω(err).Should(HaveOccurred())
					Ω(err).Should(Equal(subnets.ErrNotAllowed))
				})
			})

			Context("when the requested subnet subsumes the dynamic allocation range", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.4/30")
				})

				It("returns an appropriate error", func() {
					_, static := networkParms("10.2.3.0/24")

					_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
					Ω(err).Should(HaveOccurred())
					Ω(err).Should(Equal(subnets.ErrNotAllowed))
				})
			})

			Context("when the requested subnet is not within the dynamic allocation range", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/29")
				})

				Context("allocating a static subnet", func() {
					Context("and a static IP", func() {
						It("returns an error if the IP is not inside the subnet", func() {
							_, static := networkParms("11.0.0.0/8")

							ip := net.ParseIP("9.0.0.1")
							_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ip})
							Ω(err).Should(Equal(subnets.ErrInvalidIP))
						})

						It("returns the same subnet and IP if the IP is inside the subnet", func() {
							_, static := networkParms("11.0.0.0/8")

							ip := net.ParseIP("11.0.0.1")
							returnedSubnet, returnedIp, first, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ip})
							Ω(err).ShouldNot(HaveOccurred())

							Ω(returnedSubnet).Should(Equal(static))
							Ω(returnedIp).Should(Equal(ip))
							Ω(first).Should(BeTrue())
						})

						It("does not allow the same IP to be requested twice", func() {
							_, static := networkParms("11.0.0.0/8")

							ip := net.ParseIP("11.0.0.1")
							_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ip})
							Ω(err).ShouldNot(HaveOccurred())

							_, static = networkParms("11.0.0.0/8") // make sure we get a new pointer
							_, _, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ip})
							Ω(err).Should(Equal(subnets.ErrIPAlreadyAllocated))
						})

						It("allows two IPs to be serially requested in the same subnet", func() {
							_, static := networkParms("11.0.0.0/8")

							ip := net.ParseIP("11.0.0.1")
							returnedSubnet, returnedIp, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ip})
							Ω(err).ShouldNot(HaveOccurred())
							Ω(returnedSubnet).Should(Equal(static))
							Ω(returnedIp).Should(Equal(ip))

							ip2 := net.ParseIP("11.0.0.2")

							_, static = networkParms("11.0.0.0/8") // make sure we get a new pointer
							returnedSubnet2, returnedIp2, first, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ip2})
							Ω(err).ShouldNot(HaveOccurred())
							Ω(returnedSubnet2).Should(Equal(static))
							Ω(returnedIp2).Should(Equal(ip2))
							Ω(first).Should(BeFalse())
						})

						It("prevents dynamic allocation of the same IP", func() {
							_, static := networkParms("11.0.0.0/8")

							ip := net.ParseIP("11.0.0.2")
							_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ip})
							Ω(err).ShouldNot(HaveOccurred())

							_, ip, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())
							Ω(ip.String()).Should(Equal("11.0.0.1"))

							_, ip, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())
							Ω(ip.String()).Should(Equal("11.0.0.3"))
						})

						Describe("errors", func() {
							It("fails if a static subnet is requested specifying an IP address which clashes with the gateway IP address", func() {
								_, static := networkParms("11.0.0.0/8")
								gateway := net.ParseIP("11.255.255.254")
								_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{gateway})
								Ω(err).Should(MatchError(subnets.ErrIPEqualsGateway))
							})

							It("fails if a static subnet is requested specifying an IP address which clashes with the broadcast IP address", func() {
								_, static := networkParms("11.0.0.0/8")
								max := net.ParseIP("11.255.255.255")
								_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{max})
								Ω(err).Should(MatchError(subnets.ErrIPEqualsBroadcast))
							})
						})
					})

					Context("and a dynamic IP", func() {
						It("does not return an error", func() {
							_, static := networkParms("11.0.0.0/8")

							_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())
						})

						It("returns the first available IP", func() {
							_, static := networkParms("11.0.0.0/8")

							_, ip, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())

							Ω(ip.String()).Should(Equal("11.0.0.1"))
						})

						It("returns distinct IPs", func() {
							_, static := networkParms("11.0.0.0/22")

							seen := make(map[string]bool)
							var err error
							for err == nil {
								var ip net.IP
								_, ip, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)

								if err != nil {
									Ω(err).Should(Equal(subnets.ErrInsufficientIPs))
								}

								Ω(seen).ShouldNot(HaveKey(ip.String()))
								seen[ip.String()] = true
							}
						})

						It("returns all IPs except gateway, minimum and broadcast", func() {
							_, static := networkParms("11.0.0.0/23")

							var err error
							count := 0
							for err == nil {
								if _, _, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector); err != nil {
									Ω(err).Should(Equal(subnets.ErrInsufficientIPs))
								}

								count++
							}

							Ω(count).Should(Equal(510))
						})

						It("causes static alocation to fail if it tries to allocate the same IP afterwards", func() {
							_, static := networkParms("11.0.0.0/8")

							_, ip, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())

							_, _, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ip})
							Ω(err).Should(Equal(subnets.ErrIPAlreadyAllocated))
						})
					})
				})

				Context("after all IPs are allocated from a subnet, a subsequent request for the same subnet", func() {
					var (
						static *net.IPNet
						ips    [5]net.IP
					)

					JustBeforeEach(func() {
						var err error
						_, static, err = net.ParseCIDR("10.9.3.0/29")
						Ω(err).ShouldNot(HaveOccurred())

						for i := 0; i < 5; i++ {
							_, ips[i], _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())
						}
					})

					It("returns an appropriate error", func() {
						_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
						Ω(err).Should(HaveOccurred())
						Ω(err).Should(Equal(subnets.ErrInsufficientIPs))
					})

					Context("but after it is released", func() {
						It("dynamically allocates the released IP again", func() {
							_, err := subnetpool.Release(static, ips[3])
							Ω(err).ShouldNot(HaveOccurred())

							_, allocatedIP, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())
							Ω(allocatedIP).Should(Equal(ips[3]))
						})

						It("allows static allocation again", func() {
							_, err := subnetpool.Release(static, ips[3])
							Ω(err).ShouldNot(HaveOccurred())

							_, _, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ips[3]})
							Ω(err).ShouldNot(HaveOccurred())
						})
					})
				})

				Context("after a subnet has been allocated, a subsequent request for an overlapping subnet", func() {
					var (
						firstSubnetPool  *net.IPNet
						firstContainerIP net.IP
						secondSubnetPool *net.IPNet
					)

					JustBeforeEach(func() {
						var err error
						firstContainerIP, firstSubnetPool = networkParms("10.9.3.4/30")
						Ω(err).ShouldNot(HaveOccurred())

						_, secondSubnetPool = networkParms("10.9.3.0/29")
						Ω(err).ShouldNot(HaveOccurred())

						_, _, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{firstSubnetPool}, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())
					})

					It("returns an appropriate error", func() {
						_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{secondSubnetPool}, subnets.DynamicIPSelector)
						Ω(err).Should(HaveOccurred())
						Ω(err).Should(Equal(subnets.ErrOverlapsExistingSubnet))
					})

					Context("but after it is released", func() {
						It("allows allocation again", func() {
							gone, err := subnetpool.Release(firstSubnetPool, firstContainerIP)
							Ω(err).ShouldNot(HaveOccurred())
							Ω(gone).Should(BeTrue())

							_, _, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{secondSubnetPool}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())
						})
					})
				})

				Context("requesting a specific IP address in a static subnet", func() {
					It("does not return an error", func() {
						_, static := networkParms("10.9.3.6/29")

						_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())
					})
				})

			})
		})

		Describe("Dynamic /30 Subnet Allocation", func() {
			Context("when the pool does not have sufficient IPs to allocate a subnet", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/31")
				})

				It("the first request returns an error", func() {
					_, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when the pool has sufficient IPs to allocate a single subnet", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/30")
				})

				Context("the first request", func() {
					It("succeeds, and returns a /30 network within the subnet", func() {
						network, _, first, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())
						Ω(first).Should(BeTrue())

						Ω(network).ShouldNot(BeNil())
						Ω(network.String()).Should(Equal("10.2.3.0/30"))
					})
				})

				Context("subsequent requests", func() {
					It("fails, and return an err", func() {
						_, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())

						_, _, _, err = subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).Should(HaveOccurred())
					})
				})

				Context("when an allocated network is released", func() {
					It("a subsequent allocation succeeds, and returns the first network again", func() {
						// first
						allocated, ip, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())

						// second - will fail (sanity check)
						_, _, _, err = subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).Should(HaveOccurred())

						// release
						_, err = subnetpool.Release(allocated, ip)
						Ω(err).ShouldNot(HaveOccurred())

						// third - should work now because of release
						network, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(network).ShouldNot(BeNil())
						Ω(network.String()).Should(Equal(allocated.String()))
					})

					Context("and it is not the last IP in the subnet", func() {
						It("returns gone=false", func() {
							_, static := networkParms("10.3.3.0/29")

							_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())

							allocated, ip, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())

							gone, err := subnetpool.Release(allocated, ip)
							Ω(err).ShouldNot(HaveOccurred())
							Ω(gone).Should(BeFalse())
						})
					})

					Context("and it is the last IP in the subnet", func() {
						It("returns gone=true", func() {
							allocated, ip, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())

							gone, err := subnetpool.Release(allocated, ip)
							Ω(err).ShouldNot(HaveOccurred())
							Ω(gone).Should(BeTrue())
						})
					})
				})

				Context("when a network is released twice", func() {
					It("returns an error", func() {
						// first
						allocated, ip, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())

						// release
						_, err = subnetpool.Release(allocated, ip)
						Ω(err).ShouldNot(HaveOccurred())

						// release again
						_, err = subnetpool.Release(allocated, ip)
						Ω(err).Should(HaveOccurred())
						Ω(err).Should(Equal(subnets.ErrReleasedUnallocatedSubnet))
					})
				})
			})

			Context("when the pool has sufficient IPs to allocate two /30 subnets", func() {
				BeforeEach(func() {
					defaultSubnetPool = subnetPool("10.2.3.0/29")
				})

				Context("the second request", func() {
					It("succeeds", func() {
						_, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())

						_, _, _, err = subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())
					})

					It("returns the second /30 network within the subnet", func() {
						_, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())

						network, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
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

						subnetpool, err := subnets.NewSubnets(network)
						Ω(err).ShouldNot(HaveOccurred())

						out := make(chan *net.IPNet)
						go func(out chan *net.IPNet) {
							defer GinkgoRecover()
							n1, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
							Ω(err).ShouldNot(HaveOccurred())
							out <- n1
						}(out)

						go func(out chan *net.IPNet) {
							defer GinkgoRecover()
							n1, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
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

						subnetpool, err := subnets.NewSubnets(network)
						Ω(err).ShouldNot(HaveOccurred())

						n1, ip, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
						Ω(err).ShouldNot(HaveOccurred())

						out := make(chan error)
						go func(out chan error) {
							defer GinkgoRecover()
							_, err := subnetpool.Release(n1, ip)
							out <- err
						}(out)

						go func(out chan error) {
							defer GinkgoRecover()
							_, err := subnetpool.Release(n1, ip)
							out <- err
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

						subnetpool, err := subnets.NewSubnets(network)
						Ω(err).ShouldNot(HaveOccurred())

						ip, n1 := networkParms("10.1.0.0/30")

						out := make(chan error)
						go func(out chan error) {
							defer GinkgoRecover()
							_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{n1}, subnets.StaticIPSelector{ip})
							out <- err
						}(out)

						go func(out chan error) {
							defer GinkgoRecover()
							_, _, _, err := subnetpool.Allocate(subnets.StaticSubnetSelector{n1}, subnets.StaticIPSelector{ip})
							out <- err
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

					err := subnetpool.Recover(static, net.ParseIP("10.9.3.5"))
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("does not allow recovering twice", func() {
					_, static := networkParms("10.9.3.4/30")

					err := subnetpool.Recover(static, net.ParseIP("10.9.3.5"))
					Ω(err).ShouldNot(HaveOccurred())

					err = subnetpool.Recover(static, net.ParseIP("10.9.3.5"))
					Ω(err).Should(HaveOccurred())
				})

				It("does not allow allocating after recovery", func() {
					_, static := networkParms("10.9.3.4/30")

					ip := net.ParseIP("10.9.3.5")
					err := subnetpool.Recover(static, ip)
					Ω(err).ShouldNot(HaveOccurred())

					_, _, _, err = subnetpool.Allocate(subnets.StaticSubnetSelector{static}, subnets.StaticIPSelector{ip})
					Ω(err).Should(HaveOccurred())
				})

				It("does not allow recovering without an explicit IP", func() {
					_, static := networkParms("10.9.3.4/30")

					err := subnetpool.Recover(static, nil)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("an allocation inside the dynamic allocation net", func() {
				It("recovers the first time", func() {
					_, static := networkParms("10.2.3.4/30")

					err := subnetpool.Recover(static, net.ParseIP("10.2.3.5"))
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("does not allow recovering twice", func() {
					_, static := networkParms("10.2.3.4/30")

					err := subnetpool.Recover(static, net.ParseIP("10.2.3.5"))
					Ω(err).ShouldNot(HaveOccurred())

					err = subnetpool.Recover(static, net.ParseIP("10.2.3.5"))
					Ω(err).Should(HaveOccurred())
				})

				It("does not dynamically allocate a recovered network", func() {
					_, static := networkParms("10.2.3.4/30")

					err := subnetpool.Recover(static, net.ParseIP("10.2.3.1"))
					Ω(err).ShouldNot(HaveOccurred())

					network, _, _, err := subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.StaticIPSelector{net.ParseIP("10.2.3.1")})
					Ω(err).ShouldNot(HaveOccurred())
					Ω(network.String()).Should(Equal("10.2.3.0/30"))

					_, _, _, err = subnetpool.Allocate(subnets.DynamicSubnetSelector, subnets.StaticIPSelector{net.ParseIP("10.2.3.1")})
					Ω(err).Should(Equal(subnets.ErrInsufficientSubnets))
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
