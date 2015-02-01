package netfence

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/subnets/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/process"
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
		fakeSubnetPool   *fakes.FakeBridgedSubnets
		fence            *fenceBuilder
		syscfg           sysconfig.Config  = sysconfig.NewConfig("", false)
		sysconfig        *sysconfig.Config = &syscfg
		fakeDeconfigurer *FakeDeconfigurer
	)

	JustBeforeEach(func() {
		fence = &fenceBuilder{
			BridgedSubnets: fakeSubnetPool,
			mtu:            1500,
			externalIP:     net.ParseIP("1.2.3.4"),
			deconfigurer:   fakeDeconfigurer,
			log:            lagertest.NewTestLogger("fence"),
		}
	})

	BeforeEach(func() {
		// _, a, err := net.ParseCIDR("1.2.0.0/22")
		// Ω(err).ShouldNot(HaveOccurred())

		fakeSubnetPool = &fakes.FakeBridgedSubnets{}
		// fakeSubnetPool.AllocateReturns(a,
		// &fakeSubnets{nextSubnet: a}
		fakeDeconfigurer = &FakeDeconfigurer{}
	})

	Describe("Capacity", func() {
		BeforeEach(func() {
			fakeSubnetPool.CapacityReturns(4)
		})

		It("delegates to Subnets", func() {
			Ω(fence.Capacity()).Should(Equal(4))
		})
	})

	Describe("Build", func() {
		Context("when the network parameter is empty", func() {
			It("allocates a dynamic subnet and dynamic IP from Subnets", func() {
				_, subNet, err := net.ParseCIDR("3.4.5.0/30")
				Ω(err).ShouldNot(HaveOccurred())

				fakeSubnetPool.AllocateReturns(subNet, net.ParseIP("3.4.5.1"), "", nil)

				allocation, err := fence.Build("", sysconfig, "")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeSubnetPool.AllocateCallCount()).Should(Equal(1))
				ss, is := fakeSubnetPool.AllocateArgsForCall(0)

				Ω(ss).Should(Equal(subnets.DynamicSubnetSelector))
				Ω(allocation).Should(HaveSubnet("3.4.5.0/30"))

				Ω(is).Should(Equal(subnets.DynamicIPSelector))
				Ω(allocation).Should(HaveContainerIP("3.4.5.1"))
			})

			It("passes back an error if allocation fails", func() {
				testErr := errors.New("some error")
				fakeSubnetPool.AllocateReturns(nil, nil, "", testErr)

				_, err := fence.Build("", sysconfig, "")
				Ω(err).Should(Equal(testErr))
			})
		})

		Context("when the network parameter is not empty", func() {
			var (
				subNetString string
				ipString     string
			)
			BeforeEach(func() {
				subNetString = "1.1.1.0/24"
				ipString = "1.1.1.1"
			})

			JustBeforeEach(func() {
				_, subNet, err := net.ParseCIDR(subNetString)
				Ω(err).ShouldNot(HaveOccurred())

				fakeSubnetPool.AllocateReturns(subNet, net.ParseIP(ipString), "", nil)
			})

			Context("when it contains a prefix length", func() {
				BeforeEach(func() {
					subNetString = "1.3.4.0/28"
					ipString = "1.3.4.1"
				})
				It("statically allocates the requested subnet ", func() {
					_, err := fence.Build("1.3.4.0/28", sysconfig, "")
					Ω(err).ShouldNot(HaveOccurred())

					_, cidr, err := net.ParseCIDR("1.3.4.0/28")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeSubnetPool.AllocateCallCount()).Should(Equal(1))
					ss, _ := fakeSubnetPool.AllocateArgsForCall(0)
					Ω(ss).Should(Equal(subnets.StaticSubnetSelector{cidr}))
				})
			})

			Context("when it does not contain a prefix length", func() {
				BeforeEach(func() {
					subNetString = "1.3.4.0/30"
					ipString = "1.3.4.1"
				})

				It("statically allocates the requested Network from Subnets as a /30", func() {
					_, err := fence.Build("1.3.4.0", sysconfig, "")
					Ω(err).ShouldNot(HaveOccurred())

					_, cidr, err := net.ParseCIDR("1.3.4.0/30")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeSubnetPool.AllocateCallCount()).Should(Equal(1))
					ss, _ := fakeSubnetPool.AllocateArgsForCall(0)

					Ω(ss).Should(Equal(subnets.StaticSubnetSelector{cidr}))
				})
			})

			Context("when the network parameter has non-zero host bits", func() {
				BeforeEach(func() {
					subNetString = "1.3.4.4/30"
					ipString = "1.3.4.5"
				})

				It("statically allocates an IP address based on the network parameter", func() {
					_, err := fence.Build("1.3.4.5", sysconfig, "")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeSubnetPool.AllocateCallCount()).Should(Equal(1))
					_, is := fakeSubnetPool.AllocateArgsForCall(0)

					ip := net.ParseIP("1.3.4.5")
					Ω(is).Should(Equal(subnets.StaticIPSelector{ip}))
				})
			})

			Context("when the network parameter has zero host bits", func() {
				BeforeEach(func() {
					subNetString = "1.3.4.0/30"
					ipString = "9.8.7.6"
				})

				It("dynamically allocates an IP address", func() {
					allocation, err := fence.Build("1.3.4.0", sysconfig, "")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeSubnetPool.AllocateCallCount()).Should(Equal(1))
					_, is := fakeSubnetPool.AllocateArgsForCall(0)

					Ω(is).Should(Equal(subnets.DynamicIPSelector))
					Ω(allocation).Should(HaveContainerIP("9.8.7.6"))
				})
			})

			It("returns an error if an invalid network string is passed", func() {
				_, err := fence.Build("invalid", sysconfig, "")
				Ω(err).Should(HaveOccurred())
				Ω(fakeSubnetPool.AllocateCallCount()).Should(Equal(0))
			})

			It("returns an error if allocation fails", func() {
				testErr := errors.New("some error")
				fakeSubnetPool.AllocateReturns(nil, nil, "", testErr)

				_, err := fence.Build("1.3.4.4/30", sysconfig, "")
				Ω(err).Should(Equal(testErr))
				Ω(fakeSubnetPool.AllocateCallCount()).Should(Equal(1))
			})
		})

	})

	var allocate = func(subnet, ip string) *Fence {
		_, s, err := net.ParseCIDR(subnet)
		Ω(err).ShouldNot(HaveOccurred())

		return &Fence{s, net.ParseIP(ip), "cIfc", "host", false, "bridge", fence, lagertest.NewTestLogger("allocation")}
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
				Ω(err).ShouldNot(HaveOccurred())

				original := allocate("1.2.0.5/28", "1.2.0.5")

				md, err = original.MarshalJSON()
				Ω(err).ShouldNot(HaveOccurred())

				recovered, err := fence.Rebuild(&md)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeSubnetPool.RecoverCallCount()).Should(Equal(1))
				rSubnet, rIp, rBridgeIfcName := fakeSubnetPool.RecoverArgsForCall(0)

				Ω(rSubnet).Should(Equal(s))
				Ω(rIp).Should(Equal(ip))
				Ω(rBridgeIfcName).Should(Equal("bridge"))

				recoveredAllocation := recovered.(*Fence)

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

				testErr := errors.New("o no")
				fakeSubnetPool.RecoverReturns(testErr)

				_, err = fence.Rebuild(&md)
				Ω(err).Should(Equal(testErr))
			})
		})
	})

	Describe("Allocations return by Allocate", func() {
		Describe("Dismantle", func() {
			Context("when releasing the in-memory allocation fails", func() {
				It("returns an error", func() {
					testErr := errors.New("o no")
					fakeSubnetPool.ReleaseReturns(false, "", testErr)

					Ω(allocate("1.2.0.0/22", "1.2.0.1").Dismantle()).Should(Equal(testErr))

					By("and does not attempt to destroy the bridge")
					Ω(fakeDeconfigurer.DeconfiguredBridges).Should(HaveLen(0))
				})
			})

			Context("when the IP is not the final IP in the subnet", func() {
				var (
					allocation *Fence
				)

				JustBeforeEach(func() {
					fakeSubnetPool.ReleaseReturns(false, "bridge", nil)

					allocation = allocate("1.2.0.0/22", "1.2.0.1")
				})

				It("releases the IP in the subnet", func() {
					Ω(allocation.Dismantle()).Should(Succeed())

					Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(1))
					ipNet, ip := fakeSubnetPool.ReleaseArgsForCall(0)
					Ω(ipNet.String()).Should(Equal("1.2.0.0/22"))
					Ω(ip.String()).Should(Equal("1.2.0.1"))

					By("and does not attempt to destroy the bridge")
					Ω(fakeDeconfigurer.DeconfiguredBridges).Should(HaveLen(0))
				})
			})

			Context("when the final IP in the subnet is released", func() {
				var (
					allocation *Fence
				)

				JustBeforeEach(func() {
					fakeSubnetPool.ReleaseReturns(true, "bridge", nil)

					allocation = allocate("1.2.0.0/22", "1.2.0.1")
				})

				It("releases the IP in the subnet", func() {
					Ω(allocation.Dismantle()).Should(Succeed())

					Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(1))
					ipNet, ip := fakeSubnetPool.ReleaseArgsForCall(0)
					Ω(ipNet.String()).Should(Equal("1.2.0.0/22"))
					Ω(ip.String()).Should(Equal("1.2.0.1"))

					By("and destroys the bridge")
					Ω(fakeDeconfigurer.DeconfiguredBridges).Should(ContainElement("bridge"))
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
					env process.Env
				)

				JustBeforeEach(func() {
					_, ipn, err := net.ParseCIDR("4.5.6.0/29")
					Ω(err).ShouldNot(HaveOccurred())

					fence.mtu = 123

					env = process.Env{"foo": "bar"}
					allocation := &Fence{ipn, net.ParseIP("4.5.6.1"), "", "host", false, "bridge", fence, lagertest.NewTestLogger("allocation")}
					allocation.ConfigureProcess(env)
				})

				It("configures with the correct network_cidr", func() {
					Ω(env.Array()).Should(ContainElement("network_cidr=4.5.6.0/29"))
				})

				It("configures with the correct gateway ip", func() {
					Ω(env.Array()).Should(ContainElement("network_host_ip=4.5.6.6"))
				})

				It("configures with the correct container ip", func() {
					Ω(env.Array()).Should(ContainElement("network_container_ip=4.5.6.1"))
				})

				It("configures with the correct cidr suffix", func() {
					Ω(env.Array()).Should(ContainElement("network_cidr_suffix=29"))
				})

				It("configures with the correct MTU size", func() {
					Ω(env.Array()).Should(ContainElement("container_iface_mtu=123"))
				})

				It("configures with the correct external IP", func() {
					Ω(env.Array()).Should(ContainElement("external_ip=1.2.3.4"))
				})
			})
		})
	})
})

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
