package netfence

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

type FakeDeconfigurer struct {
	DeconfiguredBridges []string
	DestroyReturns      error
}

func (f *FakeDeconfigurer) DeconfigureBridge(logger lager.Logger, bridgeIfc string) error {
	f.DeconfiguredBridges = append(f.DeconfiguredBridges, bridgeIfc)
	return f.DestroyReturns
}

var _ = Describe("Fence", func() {
	var (
		fakeSubnetPool   *fakeSubnets
		fence            *fenceBuilder
		syscfg           sysconfig.Config  = sysconfig.NewConfig("", false)
		sysconfig        *sysconfig.Config = &syscfg
		fakeDeconfigurer *FakeDeconfigurer
	)

	BeforeEach(func() {
		_, a, err := net.ParseCIDR("1.2.0.0/22")
		Ω(err).ShouldNot(HaveOccurred())

		fakeSubnetPool = &fakeSubnets{nextSubnet: a}
		fakeDeconfigurer = &FakeDeconfigurer{}
		fence = &fenceBuilder{
			Subnets:      fakeSubnetPool,
			mtu:          1500,
			externalIP:   net.ParseIP("1.2.3.4"),
			deconfigurer: fakeDeconfigurer,
			log:          lagertest.NewTestLogger("fence"),
		}
	})

	Describe("Capacity", func() {
		It("delegates to Subnets", func() {
			fakeSubnetPool.capacity = 4
			fence := &fenceBuilder{
				Subnets:      fakeSubnetPool,
				mtu:          1500,
				externalIP:   net.ParseIP("1.2.3.4"),
				deconfigurer: fakeDeconfigurer,
				log:          lagertest.NewTestLogger("fence"),
			}

			Ω(fence.Capacity()).Should(Equal(4))
		})
	})

	Describe("Build", func() {
		Context("when the network parameter is empty", func() {
			It("allocates a dynamic subnet from Subnets", func() {
				var err error
				_, fakeSubnetPool.nextSubnet, err = net.ParseCIDR("3.4.5.0/30")
				Ω(err).ShouldNot(HaveOccurred())

				allocation, err := fence.Build("", sysconfig, "")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeSubnetPool.lastRequested.Subnet).Should(Equal(subnets.DynamicSubnetSelector))
				Ω(allocation).Should(HaveSubnet("3.4.5.0/30"))
			})

			It("allocates a dynamic IP from Subnets", func() {
				fakeSubnetPool.nextIP = net.ParseIP("2.2.3.3")

				allocation, err := fence.Build("", sysconfig, "")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeSubnetPool.lastRequested.IP).Should(Equal(subnets.DynamicIPSelector))
				Ω(allocation).Should(HaveContainerIP("2.2.3.3"))
			})

			It("passes back an error if allocation fails", func() {
				testErr := errors.New("some error")
				fakeSubnetPool.allocationError = testErr

				_, err := fence.Build("", sysconfig, "")
				Ω(err).Should(Equal(testErr))
			})
		})

		Context("when the network parameter is not empty", func() {
			Context("when it contains a prefix length", func() {
				It("statically allocates the requested subnet ", func() {
					_, err := fence.Build("1.3.4.0/28", sysconfig, "")
					Ω(err).ShouldNot(HaveOccurred())

					_, cidr, err := net.ParseCIDR("1.3.4.0/28")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeSubnetPool.lastRequested.Subnet).Should(Equal(subnets.StaticSubnetSelector{cidr}))
				})
			})

			Context("when it does not contain a prefix length", func() {
				It("statically allocates the requested Network from Subnets as a /30", func() {
					_, err := fence.Build("1.3.4.0", sysconfig, "")
					Ω(err).ShouldNot(HaveOccurred())

					_, cidr, err := net.ParseCIDR("1.3.4.0/30")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeSubnetPool.lastRequested.Subnet).Should(Equal(subnets.StaticSubnetSelector{cidr}))
				})
			})

			Context("when the network parameter has non-zero host bits", func() {
				It("statically allocates an IP address based on the network parameter", func() {
					_, err := fence.Build("1.3.4.2", sysconfig, "")
					Ω(err).ShouldNot(HaveOccurred())

					ip := net.ParseIP("1.3.4.2")
					Ω(fakeSubnetPool.lastRequested.IP).Should(Equal(subnets.StaticIPSelector{ip}))
				})
			})

			Context("when the network parameter has zero host bits", func() {
				It("dynamically allocates an IP address", func() {
					fakeSubnetPool.nextIP = net.ParseIP("9.8.7.6")

					allocation, err := fence.Build("1.3.4.0", sysconfig, "")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeSubnetPool.lastRequested.IP).Should(Equal(subnets.DynamicIPSelector))
					Ω(allocation).Should(HaveContainerIP("9.8.7.6"))
				})
			})

			It("returns an error if an invalid network string is passed", func() {
				_, err := fence.Build("invalid", sysconfig, "")
				Ω(err).Should(HaveOccurred())
			})

			It("returns an error if allocation fails", func() {
				testErr := errors.New("some error")
				fakeSubnetPool.allocationError = testErr

				_, err := fence.Build("1.3.4.4/30", sysconfig, "")
				Ω(err).Should(Equal(testErr))
			})

		})
	})

	var allocate = func(subnet, ip string) *Fence {
		_, s, err := net.ParseCIDR(subnet)
		Ω(err).ShouldNot(HaveOccurred())

		return &Fence{s, net.ParseIP(ip), "", "host", false, "bridge", fence, lagertest.NewTestLogger("allocation")}
	}

	It("correctly Strings Allocation instances", func() {
		a := allocate("9.8.7.6/27", "1.2.3.4")
		Ω(a.String()).Should(HavePrefix("netfence.Fence{IPNet:"))
	})

	Describe("Rebuild", func() {
		Context("When there is not an error", func() {
			It("parses the message from JSON, delegates to Subnets, and rebuilds the fence correctly", func() {
				var err error
				var md json.RawMessage

				ip, s, err := net.ParseCIDR("1.2.0.5/28")
				original := &Fence{s, ip, "", "foo", false, "bridge", fence, nil}
				Ω(err).ShouldNot(HaveOccurred())
				md, err = original.MarshalJSON()
				Ω(err).ShouldNot(HaveOccurred())

				recovered, err := fence.Rebuild(&md)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(fakeSubnetPool.recovered).Should(ContainElement(fakeAllocation{"1.2.0.0/28", "1.2.0.5"}))

				recoveredAllocation := recovered.(*Fence)
				Ω(recoveredAllocation.IPNet.String()).Should(Equal("1.2.0.0/28"))
				Ω(recoveredAllocation.containerIP.String()).Should(Equal("1.2.0.5"))

				recoveredAllocation.fenceBldr = nil
				original.fenceBldr = nil
				recoveredAllocation.log = nil
				original.log = nil
				Ω(recoveredAllocation).Should(Equal(original))
			})
		})

		Context("when the subnetPool returns an error", func() {
			It("passes the error back", func() {
				var err error
				var md json.RawMessage
				md, err = allocate("1.2.0.0/22", "1.2.0.1").MarshalJSON()
				Ω(err).ShouldNot(HaveOccurred())

				fakeSubnetPool.recoverError = errors.New("o no")

				_, err = fence.Rebuild(&md)
				Ω(err).Should(MatchError("o no"))
			})
		})
	})

	Describe("Allocations return by Allocate", func() {
		Describe("Dismantle", func() {
			Context("when releasing the in-memory allocation fails", func() {
				var (
					allocation *Fence
				)

				BeforeEach(func() {
					allocation = allocate("1.2.0.0/22", "1.2.0.1")
					allocation.hostIfc = "thehost"
					allocation.bridgeIfc = "thebridge"

					fakeSubnetPool.releaseError = errors.New("o no")
				})

				It("returns an error", func() {
					err := allocation.Dismantle()
					Ω(err).Should(HaveOccurred())
				})

				It("does not attempt to destroy the bridge", func() {
					Ω(fakeDeconfigurer.DeconfiguredBridges).Should(HaveLen(0))
				})
			})

			Context("when the IP is not the final IP in the subnet", func() {
				var (
					allocation *Fence
				)

				BeforeEach(func() {
					allocation = allocate("1.2.0.0/22", "1.2.0.1")
					allocation.hostIfc = "thehost"
					allocation.bridgeIfc = "thebridge"

					fakeSubnetPool.releaseReturns = false
				})

				It("releases the subnet", func() {
					err := allocation.Dismantle()
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeSubnetPool.released).Should(ContainElement(fakeAllocation{"1.2.0.0/22", "1.2.0.1"}))
				})

				It("does not destroy the bridge", func() {
					err := allocation.Dismantle()
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeDeconfigurer.DeconfiguredBridges).ShouldNot(ContainElement("thebridge"))
				})
			})

			Context("when the final IP in the subnet is released", func() {
				var (
					allocation *Fence
				)

				BeforeEach(func() {
					fakeSubnetPool.releaseReturns = true
					allocation = allocate("1.2.0.0/22", "1.2.0.1")

					allocation.hostIfc = "thehost"
					allocation.bridgeIfc = "thebridge"
				})

				It("releases the subnet", func() {
					err := allocation.Dismantle()
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeSubnetPool.released).Should(ContainElement(fakeAllocation{"1.2.0.0/22", "1.2.0.1"}))
				})

				It("destroys the bridge", func() {
					err := allocation.Dismantle()
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeDeconfigurer.DeconfiguredBridges).Should(ContainElement("thebridge"))
				})
			})
		})

		Describe("Info", func() {
			It("stores network info of a /30 subnet in the container api object", func() {
				allocation := allocate("1.2.0.0/30", "9.8.7.6")
				var garden garden.ContainerInfo
				allocation.Info(&garden)

				Ω(garden.HostIP).Should(Equal("1.2.0.2"))
				Ω(garden.ContainerIP).Should(Equal("9.8.7.6"))
			})

			It("stores network info of a /28 subnet with a specified IP in the container api object", func() {
				allocation := allocate("1.2.0.5/28", "9.8.7.6")
				var garden garden.ContainerInfo
				allocation.Info(&garden)

				Ω(garden.HostIP).Should(Equal("1.2.0.14"))
				Ω(garden.ContainerIP).Should(Equal("9.8.7.6"))
			})
		})

		Describe("ConfigureProcess", func() {
			Context("With a /29", func() {
				var (
					env []string
				)

				BeforeEach(func() {
					_, ipn, err := net.ParseCIDR("4.5.6.0/29")
					Ω(err).ShouldNot(HaveOccurred())

					fence.mtu = 123

					env = []string{"foo", "bar"}
					allocation := &Fence{ipn, net.ParseIP("4.5.6.1"), "", "host", false, "bridge", fence, lagertest.NewTestLogger("allocation")}
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

				It("configures with the correct external IP", func() {
					Ω(env).Should(ContainElement("external_ip=1.2.3.4"))
				})
			})
		})
	})
})

type fakeSubnets struct {
	nextSubnet      *net.IPNet
	nextIP          net.IP
	allocationError error
	lastRequested   struct {
		Subnet subnets.SubnetSelector
		IP     subnets.IPSelector
	}

	released  []fakeAllocation
	recovered []fakeAllocation
	capacity  int

	releaseReturns bool

	releaseError error
	recoverError error
}

type fakeAllocation struct {
	subnet      string
	containerIP string
}

func (f *fakeSubnets) Allocate(s subnets.SubnetSelector, i subnets.IPSelector) (*net.IPNet, net.IP, error) {
	if f.allocationError != nil {
		return nil, nil, f.allocationError
	}

	f.lastRequested.Subnet = s
	f.lastRequested.IP = i

	return f.nextSubnet, f.nextIP, nil
}

func (f *fakeSubnets) Release(n *net.IPNet, c net.IP) (bool, error) {
	f.released = append(f.released, fakeAllocation{n.String(), c.String()})
	return f.releaseReturns, f.releaseError
}

func (f *fakeSubnets) Recover(n *net.IPNet, c net.IP) error {
	f.recovered = append(f.recovered, fakeAllocation{n.String(), c.String()})
	return f.recoverError
}

func (f *fakeSubnets) Capacity() int {
	return f.capacity
}

type FakeInterface struct {
	Name           string
	DestroyReturns error
	Destroyed      bool
}

func (f *FakeInterface) Destroy() error {
	f.Destroyed = true
	return f.DestroyReturns
}

func (f *FakeInterface) String() string {
	return f.Name
}

type m struct {
	value string
	field string
}

func HaveSubnet(subnet string) *m {
	return &m{subnet, "subnet"}
}

func HaveContainerIP(ip string) *m {
	return &m{ip, "containerIP"}
}

func (m *m) Match(actual interface{}) (success bool, err error) {
	switch m.field {
	case "subnet":
		return Equal(actual.(*Fence).IPNet.String()).Match(m.value)
	case "containerIP":
		return Equal(actual.(*Fence).containerIP.String()).Match(m.value)
	}

	panic(fmt.Sprintf("unknown match type: %s", m.field))
}

func (m *m) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected %s to have %s %s", actual, m.field, m.value)
}

func (m *m) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected %s not to have %s %s", actual, m.field, m.value)
}
