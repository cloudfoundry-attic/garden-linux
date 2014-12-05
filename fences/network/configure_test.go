package network_test

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/fences/network"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var testError = errors.New("test error")

var _ = Describe("Configure", func() {
	Describe("ConfigureHost", func() {
		var (
			vethCreater    *FakeVethCreater
			linkConfigurer *FakeLink
			bridger        *FakeBridge

			configurer *network.Configurer
		)

		BeforeEach(func() {
			vethCreater = &FakeVethCreater{}
			linkConfigurer = &FakeLink{}
			bridger = &FakeBridge{}
			configurer = &network.Configurer{Veth: vethCreater, Link: linkConfigurer, Bridge: bridger}
		})

		It("creates a virtual ethernet pair", func() {
			Ω(configurer.ConfigureHost("host", "container", "bridge", 1, net.ParseIP("1.2.3.4"), nil, 1500)).Should(Succeed())

			Ω(vethCreater.CreateCalledWith.hostIfcName).Should(Equal("host"))
			Ω(vethCreater.CreateCalledWith.containerIfcName).Should(Equal("container"))
		})

		Context("when creating the pair fails", func() {
			It("returns a wrapped error", func() {
				vethCreater.CreateReturns.err = errors.New("foo bar baz")
				err := configurer.ConfigureHost("host", "container", "bridge", 1, net.ParseIP("1.2.3.4"), nil, 1500)
				Ω(err).Should(HaveOccurred())
				Ω(err).Should(MatchError(&network.VethPairCreationError{vethCreater.CreateReturns.err, "host", "container"}))
			})
		})

		Context("when creating the pair succeeds", func() {
			BeforeEach(func() {
				vethCreater.CreateReturns.host = &net.Interface{Name: "the-host"}
				vethCreater.CreateReturns.container = &net.Interface{Name: "the-container"}
			})

			It("should set mtu on the host interface", func() {
				Ω(configurer.ConfigureHost("host", "container", "bridge", 1, net.ParseIP("1.2.3.4"), nil, 123)).Should(Succeed())

				Ω(linkConfigurer.SetMTUCalledWith.Interface).Should(Equal(vethCreater.CreateReturns.host))
				Ω(linkConfigurer.SetMTUCalledWith.MTU).Should(Equal(123))
			})

			Context("When setting the mtu fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.SetMTUReturns = errors.New("o no")
					err := configurer.ConfigureHost("host", "container", "bridge", 1, net.ParseIP("1.2.3.4"), nil, 14)
					Ω(err).Should(MatchError(&network.MTUError{linkConfigurer.SetMTUReturns, vethCreater.CreateReturns.host, 14}))
				})
			})

			It("should move the container interface in to the container's namespace", func() {
				Ω(configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.4"), nil, 123)).Should(Succeed())
				Ω(linkConfigurer.SetNsCalledWith.Pid).Should(Equal(3))
			})

			Context("When moving the container interface into the namespace fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.SetNsReturns = errors.New("o no")
					err := configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.4"), nil, 14)
					Ω(err).Should(MatchError(&network.SetNsFailedError{linkConfigurer.SetNsReturns, vethCreater.CreateReturns.container, 3}))
				})
			})

			Describe("creating the bridge", func() {
				Context("when an interface of the same name does not already exist", func() {
					It("creates a bridge with the current IP and subnet", func() {
						_, subnet, _ := net.ParseCIDR("1.2.3.0/30")
						Ω(configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.1"), subnet, 123)).Should(Succeed())

						Ω(bridger.CreateCalledWith.Name).Should(Equal("bridge"))
						Ω(bridger.CreateCalledWith.IP).Should(Equal(net.ParseIP("1.2.3.1")))
						Ω(bridger.CreateCalledWith.Subnet).Should(Equal(subnet))
					})

					Context("when creating the bridge fails", func() {
						It("returns a wrapped error", func() {
							_, subnet, _ := net.ParseCIDR("1.2.3.0/30")
							bridger.CreateReturns.Error = errors.New("what happened to this cake?")
							err := configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.1"), subnet, 14)
							Ω(err).Should(MatchError(&network.BridgeCreationError{bridger.CreateReturns.Error, "bridge", net.ParseIP("1.2.3.1"), subnet}))
						})
					})

					Context("when creating the bridge succeeds", func() {
						BeforeEach(func() {
							bridger.CreateReturns.Interface = &net.Interface{Name: "the-bridge"}
						})

						It("adds the host interface to the bridge", func() {
							Ω(configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.2"), nil, 14)).Should(Succeed())
							Ω(bridger.AddCalledWith.Bridge).Should(Equal(bridger.CreateReturns.Interface))
						})

						Context("when bringing the bridge up fails", func() {
							It("returns a wrapped error", func() {
								bridger.AddReturns = errors.New("is it a bird?")
								err := configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.1"), nil, 14)
								Ω(err).Should(MatchError(&network.AddToBridgeError{bridger.AddReturns, bridger.CreateReturns.Interface, vethCreater.CreateReturns.host}))
							})
						})

						It("brings the bridge interface up", func() {
							Ω(configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.1"), nil, 14)).Should(Succeed())
							Ω(linkConfigurer.SetUpCalledWith).Should(ContainElement(bridger.CreateReturns.Interface))
						})

						Context("when bringing the bridge up fails", func() {
							It("returns a wrapped error", func() {
								cause := errors.New("there's jam in this sandwich and it's not ok")
								linkConfigurer.SetUpFunc = func(intf *net.Interface) error {
									if bridger.CreateReturns.Interface == intf {
										return cause
									}

									return nil
								}

								err := configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.1"), nil, 14)
								Ω(err).Should(MatchError(&network.LinkUpError{cause, bridger.CreateReturns.Interface, "bridge"}))
							})
						})
					})
				})

				Context("when an interface with the same name already exists", func() {
					var (
						existingBridge *net.Interface
					)

					BeforeEach(func() {
						existingBridge = &net.Interface{Name: "bridge"}
					})

					JustBeforeEach(func() {
						linkConfigurer.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
							if name == "bridge" {
								return existingBridge, true, nil
							}

							return nil, false, nil
						}
					})

					It("adds the host interface to the existing bridge", func() {
						Ω(configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.1"), nil, 14)).Should(Succeed())
						Ω(bridger.AddCalledWith.Bridge).Should(Equal(existingBridge))
					})
				})
			})

			It("brings the host interface up", func() {
				Ω(configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.1"), nil, 14)).Should(Succeed())
				Ω(linkConfigurer.SetUpCalledWith).Should(ContainElement(vethCreater.CreateReturns.host))
			})

			Context("when bringing the host interface up fails", func() {
				It("returns a wrapped error", func() {
					cause := errors.New("there's jam in this sandwich and it's not ok")
					linkConfigurer.SetUpFunc = func(intf *net.Interface) error {
						if vethCreater.CreateReturns.host == intf {
							return cause
						}

						return nil
					}

					err := configurer.ConfigureHost("host", "container", "bridge", 3, net.ParseIP("1.2.3.1"), nil, 14)
					Ω(err).Should(MatchError(&network.LinkUpError{cause, vethCreater.CreateReturns.host, "host"}))
				})
			})
		})
	})

	Describe("ConfigureContainer", func() {

		var (
			containerInterfaceName string
			containerIP            net.IP
			gatewayIP              net.IP
			subnet                 *net.IPNet
			mtu                    int
		)

		BeforeEach(func() {
			containerInterfaceName = "testCifName"
			containerIP = net.ParseIP("1.2.3.5")
			gatewayIP = net.ParseIP("1.2.3.6")
			_, subnet, _ = net.ParseCIDR("1.2.3.4/30")
			mtu = 1234
		})

		Context("when invalid parameters are specified", func() {

			It("returns an error when an empty container interface name is provided", func() {
				containerInterfaceName = ""
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrContainerInterfaceMissing))
			})

			It("returns an error when the container IP is not in the subnet", func() {
				containerIP = net.ParseIP("1.1.1.1")
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrInvalidContainerIP))
			})

			It("returns an error when the container IP is the network IP", func() {
				containerIP = net.ParseIP("1.2.3.4")
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrInvalidContainerIP))
			})

			It("returns an error when the gateway IP is the broadcast address", func() {
				gatewayIP = net.ParseIP("1.2.3.7")
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrInvalidGatewayIP))
			})

			It("returns an error when the gateway IP is not in the subnet", func() {
				gatewayIP = net.ParseIP("1.1.1.1")
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrInvalidGatewayIP))
			})

			It("returns an error when the gateway IP is the network IP", func() {
				gatewayIP = net.ParseIP("1.2.3.4")
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrInvalidGatewayIP))
			})

			It("returns an error when the gateway IP is the broadcast address", func() {
				gatewayIP = net.ParseIP("1.2.3.7")
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrInvalidGatewayIP))
			})

			It("returns an error when the container IP equals the gateway IP", func() {
				gatewayIP = containerIP
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrConflictingIPs))
			})

			It("returns an error when a subnet is not provided", func() {
				subnet = nil
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrSubnetNil))
			})

			It("returns an error when an invalid MTU size is provided", func() {
				mtu = 0
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrInvalidMtu))
			})
		})

		Context("with stubbed dependencies", func() {

			var (
				stubInterfaceByName     *stub
				stubInterfaceByNameName string
				stubInterfaceByNameIfc  *net.Interface

				stubNetworkSetMTU    *stub
				stubNetworkSetMTUIfc *net.Interface
				stubNetworkSetMTUMtu int

				stubNetworkLinkAddIp            *stub
				stubNetworkLinkAddIpIfc         *net.Interface
				stubNetworkLinkAddIpContainerIP net.IP
				stubNetworkLinkAddIpsubnet      *net.IPNet

				stubAddDefaultGw        *stub
				stubAddDefaultGwIP      string
				stubAddDefaultGwIfcName string

				stubNetworkLinkUp    *stub
				stubNetworkLinkUpIfc *net.Interface
			)

			BeforeEach(func() {
				stubInterfaceByName = errAt(nil, 0)
				stubInterfaceByNameName = ""
				stubInterfaceByNameIfc = &net.Interface{}
				network.InterfaceByName = func(name string) (*net.Interface, error) {
					stubInterfaceByNameName = name
					return stubInterfaceByNameIfc, stubInterfaceByName.call()
				}

				stubNetworkSetMTU = errAt(nil, 0)
				stubNetworkSetMTUIfc = nil
				stubNetworkSetMTUMtu = 0
				network.NetworkSetMTU = func(iface *net.Interface, mtu int) error {
					stubNetworkSetMTUIfc = iface
					stubNetworkSetMTUMtu = mtu
					return stubNetworkSetMTU.call()
				}

				stubNetworkLinkAddIp = errAt(nil, 0)
				stubNetworkLinkAddIpIfc = nil
				stubNetworkLinkAddIpContainerIP = net.ParseIP("0.0.0.0")
				stubNetworkLinkAddIpsubnet = nil
				network.NetworkLinkAddIp = func(iface *net.Interface, ip net.IP, ipNet *net.IPNet) error {
					stubNetworkLinkAddIpIfc = iface
					stubNetworkLinkAddIpContainerIP = ip
					stubNetworkLinkAddIpsubnet = ipNet
					return stubNetworkLinkAddIp.call()
				}

				stubAddDefaultGw = errAt(nil, 0)
				stubAddDefaultGwIP = ""
				stubAddDefaultGwIfcName = ""
				network.AddDefaultGw = func(ip, device string) error {
					stubAddDefaultGwIP = ip
					stubAddDefaultGwIfcName = device
					return stubAddDefaultGw.call()
				}

				stubNetworkLinkUp = errAt(nil, 0)
				stubNetworkLinkUpIfc = nil
				network.NetworkLinkUp = func(iface *net.Interface) error {
					stubNetworkLinkUpIfc = iface
					return stubNetworkLinkUp.call()
				}
			})

			It("passes the correct parameters to InterfaceByName", func() {
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(stubInterfaceByNameName).Should(Equal(containerInterfaceName))
			})

			It("returns an error when InterfaceByName fails for loopback", func() {
				stubInterfaceByName = errAt(testError, 1)
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrBadLoopbackInterface))
			})

			It("returns an error when InterfaceByName fails", func() {
				stubInterfaceByName = errAt(testError, 2)
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrBadContainerInterface))
			})

			It("passes the correct parameters to NetworkSetMTU", func() {
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(stubNetworkSetMTUIfc).Should(Equal(stubInterfaceByNameIfc))
				Ω(stubNetworkSetMTUMtu).Should(Equal(mtu))
			})

			It("returns an error when NetworkSetMTU fails", func() {
				stubNetworkSetMTU = errAt(testError, 1)
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrFailedToSetMtu))
			})

			It("passes the correct parameters to NetworkLinkAddIp", func() {
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(stubNetworkLinkAddIpIfc).Should(Equal(stubInterfaceByNameIfc))
				Ω(stubNetworkLinkAddIpContainerIP).Should(Equal(containerIP))
				Ω(stubNetworkLinkAddIpsubnet).Should(Equal(subnet))
			})

			It("returns an error when NetworkLinkAddIp fails for loopback", func() {
				stubNetworkLinkAddIp = errAt(testError, 1)
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrFailedToAddLoopbackIp))
			})

			It("returns an error when NetworkLinkAddIp fails", func() {
				stubNetworkLinkAddIp = errAt(testError, 2)
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrFailedToAddIp))
			})

			It("passes the correct parameters to AddDefaultGw", func() {
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(stubAddDefaultGwIP).Should(Equal(gatewayIP.String()))
				Ω(stubAddDefaultGwIfcName).Should(Equal(containerInterfaceName))
			})

			It("returns an error when AddDefaultGw fails", func() {
				stubAddDefaultGw = errAt(testError, 1)
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrFailedToAddGateway))
			})

			It("passes the correct parameters to NetworkLinkUp", func() {
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(stubNetworkLinkUpIfc).Should(Equal(stubInterfaceByNameIfc))
			})

			It("returns an error when NetworkLinkUp fails for loopback", func() {
				stubNetworkLinkUp = errAt(testError, 1)
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrFailedToLinkUpLoopback))
			})

			It("returns an error when NetworkLinkUp fails", func() {
				stubNetworkLinkUp = errAt(testError, 2)
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrFailedToLinkUp))
			})
		})
	})
})

func errAt(err error, at int) *stub {
	return &stub{failOnCount: at, err: err}
}

type stub struct {
	err         error
	failOnCount int
	count       int
}

func (s *stub) call() error {
	s.count++
	if s.err != nil && s.count == s.failOnCount {
		s.failOnCount = s.count + 1
		return s.err
	}
	return nil
}

type FakeVethCreater struct {
	CreateCalledWith struct {
		hostIfcName, containerIfcName string
	}

	CreateReturns struct {
		host, container *net.Interface
		err             error
	}
}

func (f *FakeVethCreater) Create(hostIfcName, containerIfcName string) (*net.Interface, *net.Interface, error) {
	f.CreateCalledWith.hostIfcName = hostIfcName
	f.CreateCalledWith.containerIfcName = containerIfcName

	return f.CreateReturns.host, f.CreateReturns.container, f.CreateReturns.err
}

type FakeLink struct {
	SetUpCalledWith []*net.Interface

	SetMTUCalledWith struct {
		Interface *net.Interface
		MTU       int
	}

	SetNsCalledWith struct {
		Interface *net.Interface
		Pid       int
	}

	SetUpFunc           func(*net.Interface) error
	InterfaceByNameFunc func(string) (*net.Interface, bool, error)

	SetMTUReturns error
	SetNsReturns  error
}

func (f *FakeLink) SetUp(intf *net.Interface) error {
	f.SetUpCalledWith = append(f.SetUpCalledWith, intf)
	if f.SetUpFunc == nil {
		return nil
	}

	return f.SetUpFunc(intf)
}

func (f *FakeLink) SetMTU(intf *net.Interface, mtu int) error {
	f.SetMTUCalledWith.Interface = intf
	f.SetMTUCalledWith.MTU = mtu
	return f.SetMTUReturns
}

func (f *FakeLink) SetNs(intf *net.Interface, pid int) error {
	f.SetNsCalledWith.Interface = intf
	f.SetNsCalledWith.Pid = pid
	return f.SetNsReturns
}

func (f *FakeLink) InterfaceByName(name string) (*net.Interface, bool, error) {
	if f.InterfaceByNameFunc != nil {
		return f.InterfaceByNameFunc(name)
	}

	return nil, false, nil
}

type FakeBridge struct {
	CreateCalledWith struct {
		Name   string
		IP     net.IP
		Subnet *net.IPNet
	}

	CreateReturns struct {
		Interface *net.Interface
		Error     error
	}

	AddCalledWith struct {
		Bridge, Slave *net.Interface
	}

	AddReturns error
}

func (f *FakeBridge) Create(name string, ip net.IP, subnet *net.IPNet) (*net.Interface, error) {
	f.CreateCalledWith.Name = name
	f.CreateCalledWith.IP = ip
	f.CreateCalledWith.Subnet = subnet
	return f.CreateReturns.Interface, f.CreateReturns.Error
}

func (f *FakeBridge) Add(bridge, slave *net.Interface) error {
	f.AddCalledWith.Bridge = bridge
	f.AddCalledWith.Slave = slave
	return f.AddReturns
}
