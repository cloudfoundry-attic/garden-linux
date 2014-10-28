package network

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fence", func() {
	var (
		fakeSubnetPool *fakeSubnets
		fence          *f
	)

	BeforeEach(func() {
		_, a, err := net.ParseCIDR("1.2.0.0/22")
		Ω(err).ShouldNot(HaveOccurred())

		fakeSubnetPool = &fakeSubnets{nextDynamicAllocation: a}
		fence = &f{fakeSubnetPool, 1500}
	})

	Describe("Capacity", func() {
		It("delegates to Subnets", func() {
			fakeSubnetPool.capacity = 4
			fence := &f{fakeSubnetPool, 1500}

			Ω(fence.Capacity()).Should(Equal(4))
		})
	})

	Describe("Build", func() {
		Context("when the network parameter is not empty", func() {
			It("statically allocates the requested network if it contains a prefix length", func() {
				_, err := fence.Build("1.3.4.0/28")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeSubnetPool.allocatedStatically).Should(ContainElement("1.3.4.0/28"))
			})

			It("returns an error if an invalid network string is passed", func() {
				_, err := fence.Build("invalid")
				Ω(err).Should(HaveOccurred())
			})

			It("passes back an error if allocation fails", func() {
				testErr := errors.New("some error")
				fakeSubnetPool.allocationError = testErr

				_, err := fence.Build("1.3.4.4/30")
				Ω(err).Should(Equal(testErr))
			})

			It("statically allocates the requested Network from Subnets as a /30 if no prefix length is specified", func() {
				_, err := fence.Build("1.3.4.4")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeSubnetPool.allocatedStatically).Should(ContainElement("1.3.4.4/30"))
			})

			It("statically allocates the requested Network with a specified IP address", func() {
				f, err := fence.Build("1.3.4.3/28")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeSubnetPool.allocatedStatically).Should(ContainElement("1.3.4.0/28"))

				containerInfo := api.ContainerInfo{}
				f.Info(&containerInfo)
				Ω(containerInfo.ContainerIP).Should(Equal("1.3.4.3"))
			})

			It("fails if a static subnet is requested specifying an IP address which clashes with the gateway IP address", func() {
				_, err := fence.Build("1.3.4.14/28")
				Ω(err).Should(MatchError(ErrIPEqualsGateway))
			})

			It("fails if a static subnet is requested specifying an IP address which clashes with the broadcast IP address", func() {
				_, err := fence.Build("1.3.4.15/28")
				Ω(err).Should(MatchError(ErrIPEqualsBroadcast))
			})
		})

		Context("when the network parameter is empty", func() {
			It("allocates a dynamic subnet from Subnets", func() {
				network, err := fence.Build("")
				Ω(err).ShouldNot(HaveOccurred())

				allocation := network.(*Allocation)
				Ω(allocation.IPNet).Should(Equal(fakeSubnetPool.nextDynamicAllocation))
			})

			It("passes back an error if allocation fails", func() {
				testErr := errors.New("some error")
				fakeSubnetPool.allocationError = testErr

				_, err := fence.Build("")
				Ω(err).Should(Equal(testErr))
			})
		})
	})

	var allocate = func(ip string) *Allocation {
		_, a, err := net.ParseCIDR("1.2.0.0/22")
		Ω(err).ShouldNot(HaveOccurred())
		privateFakeSubnetPool := &fakeSubnets{nextDynamicAllocation: a}
		privateBuilder := &f{privateFakeSubnetPool, 1500}
		privateFence, err := privateBuilder.Build(ip)
		Ω(err).ShouldNot(HaveOccurred())

		allocation := privateFence.(*Allocation)
		allocation.parent = fence
		return allocation
	}

	Describe("Rebuild", func() {
		Context("When there is not an error", func() {
			It("parses the message from JSON, delegates to Subnets, and rebuilds the fence correctly", func() {
				var err error
				var md json.RawMessage
				md, err = allocate("1.2.0.5/28").MarshalJSON()
				Ω(err).ShouldNot(HaveOccurred())

				recovered, err := fence.Rebuild(&md)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(fakeSubnetPool.recovered).Should(ContainElement("1.2.0.0/28"))

				recoveredAllocation := recovered.(*Allocation)
				Ω(recoveredAllocation.IPNet.String()).Should(Equal("1.2.0.0/28"))
				Ω(recoveredAllocation.containerIP.String()).Should(Equal("1.2.0.5"))
			})
		})

		Context("when the subnetPool returns an error", func() {
			It("passes the error back", func() {
				var err error
				var md json.RawMessage
				md, err = allocate("1.2.0.0/22").MarshalJSON()
				Ω(err).ShouldNot(HaveOccurred())

				fakeSubnetPool.recoverError = errors.New("o no")

				_, err = fence.Rebuild(&md)
				Ω(err).Should(MatchError("o no"))
				Ω(fakeSubnetPool.recovered).Should(ContainElement("1.2.0.0/22"))
			})
		})
	})

	Describe("Allocations return by Allocate", func() {
		Describe("Dismantle", func() {
			It("releases the allocation", func() {
				allocation := allocate("1.2.0.0/22")

				fakeSubnetPool.releaseError = errors.New("o no")

				Ω(allocation.Dismantle()).Should(MatchError("o no"))
				Ω(fakeSubnetPool.released).Should(ContainElement("1.2.0.0/22"))
			})
		})

		Describe("Info", func() {
			It("stores network info of a /30 subnet in the container api object", func() {
				allocation := allocate("1.2.0.0/30")
				var api api.ContainerInfo
				allocation.Info(&api)

				Ω(api.HostIP).Should(Equal("1.2.0.2"))
				Ω(api.ContainerIP).Should(Equal("1.2.0.1"))
			})

			It("stores network info of a /28 subnet with a specified IP in the container api object", func() {
				allocation := allocate("1.2.0.5/28")
				var api api.ContainerInfo
				allocation.Info(&api)

				Ω(api.HostIP).Should(Equal("1.2.0.14"))
				Ω(api.ContainerIP).Should(Equal("1.2.0.5"))
			})
		})

		Describe("ConfigureProcess", func() {
			Context("With a /29", func() {
				var (
					env []string
				)

				BeforeEach(func() {
					ipAddr, ipn, err := net.ParseCIDR("4.5.6.0/29")
					Ω(err).ShouldNot(HaveOccurred())

					fence.mtu = 123

					env = []string{"foo", "bar"}
					allocation := &Allocation{ipn, nextIP(ipAddr), fence}
					allocation.ConfigureProcess(&env)
				})

				It("configures with the correct network_cidr", func() {
					Ω(env).Should(ContainElement("network_cidr=4.5.6.0/29"))
				})

				It("configures with the correct gateway ip", func() {
					Ω(env).Should(ContainElement("network_host_ip=4.5.6.6"))
				})

				It("configures with the correct container ip", func() {
					Ω(env).Should(ContainElement("network_container_ip=4.5.6.1"))
				})

				It("configures with the correct cidr suffix", func() {
					Ω(env).Should(ContainElement("network_cidr_suffix=29"))
				})

				It("configures with the correct MTU size", func() {
					Ω(env).Should(ContainElement("container_iface_mtu=123"))
				})
			})
		})
	})

})

type fakeSubnets struct {
	nextDynamicAllocation *net.IPNet
	allocationError       error
	allocatedStatically   []string
	released              []string
	recovered             []string
	capacity              int
	releaseError          error
	recoverError          error
}

func (f *fakeSubnets) AllocateDynamically() (*net.IPNet, error) {
	if f.allocationError != nil {
		return nil, f.allocationError
	}

	return f.nextDynamicAllocation, nil
}

func (f *fakeSubnets) AllocateStatically(subnet *net.IPNet) error {
	if f.allocationError != nil {
		return f.allocationError
	}

	f.allocatedStatically = append(f.allocatedStatically, subnet.String())
	return nil
}

func (f *fakeSubnets) Release(n *net.IPNet) error {
	f.released = append(f.released, n.String())
	return f.releaseError
}

func (f *fakeSubnets) Recover(n *net.IPNet) error {
	f.recovered = append(f.recovered, n.String())
	return f.recoverError
}

func (f *fakeSubnets) Capacity() int {
	return f.capacity
}
