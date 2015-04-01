package network_test

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/devices/fakedevices"
	"github.com/cloudfoundry-incubator/garden-linux/network/fakes"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Configure", func() {
	Describe("ConfigureHost", func() {
		var (
			vethCreator    *fakedevices.FaveVethCreator
			linkConfigurer *fakedevices.FakeLink
			bridger        *fakedevices.FakeBridge

			configurer     *network.NetworkConfigurer
			existingBridge *net.Interface
			config         *network.HostConfig
		)

		BeforeEach(func() {
			vethCreator = &fakedevices.FaveVethCreator{}
			linkConfigurer = &fakedevices.FakeLink{AddIPReturns: make(map[string]error)}
			bridger = &fakedevices.FakeBridge{}
			configurer = &network.NetworkConfigurer{Veth: vethCreator, Link: linkConfigurer, Bridge: bridger, Logger: lagertest.NewTestLogger("test")}

			existingBridge = &net.Interface{Name: "bridge"}

			config = &network.HostConfig{}

		})

		JustBeforeEach(func() {
			linkConfigurer.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
				if name == "bridge" {
					return existingBridge, true, nil
				}

				return nil, false, nil
			}
		})

		It("creates a virtual ethernet pair", func() {
			config.HostIntf = "host"
			config.BridgeName = "bridge"
			config.ContainerIntf = "container"
			Expect(configurer.ConfigureHost(config)).To(Succeed())

			Expect(vethCreator.CreateCalledWith.HostIfcName).To(Equal("host"))
			Expect(vethCreator.CreateCalledWith.ContainerIfcName).To(Equal("container"))
		})

		Context("when creating the pair fails", func() {
			It("returns a wrapped error", func() {
				config.HostIntf = "host"
				config.BridgeName = "bridge"
				config.ContainerIntf = "container"
				vethCreator.CreateReturns.Err = errors.New("foo bar baz")
				err := configurer.ConfigureHost(config)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(&network.VethPairCreationError{vethCreator.CreateReturns.Err, "host", "container"}))
			})
		})

		Context("when creating the pair succeeds", func() {
			BeforeEach(func() {
				vethCreator.CreateReturns.Host = &net.Interface{Name: "the-host"}
				vethCreator.CreateReturns.Container = &net.Interface{Name: "the-container"}
			})

			It("should set mtu on the host interface", func() {
				config.HostIntf = "host"
				config.BridgeName = "bridge"
				config.Mtu = 123
				Expect(configurer.ConfigureHost(config)).To(Succeed())

				Expect(linkConfigurer.SetMTUCalledWith.Interface).To(Equal(vethCreator.CreateReturns.Host))
				Expect(linkConfigurer.SetMTUCalledWith.MTU).To(Equal(123))
			})

			Context("When setting the mtu fails", func() {
				It("returns a wrapped error", func() {
					config.HostIntf = "host"
					config.BridgeName = "bridge"
					config.ContainerIntf = "container"
					config.Mtu = 14
					linkConfigurer.SetMTUReturns = errors.New("o no")
					err := configurer.ConfigureHost(config)
					Expect(err).To(MatchError(&network.MTUError{linkConfigurer.SetMTUReturns, vethCreator.CreateReturns.Host, 14}))
				})
			})

			It("should move the container interface in to the container's namespace", func() {
				config.BridgeName = "bridge"
				config.ContainerPid = 3
				Expect(configurer.ConfigureHost(config)).To(Succeed())
				Expect(linkConfigurer.SetNsCalledWith.Pid).To(Equal(3))
			})

			Context("When moving the container interface into the namespace fails", func() {
				It("returns a wrapped error", func() {
					config.BridgeName = "bridge"
					config.ContainerPid = 3
					linkConfigurer.SetNsReturns = errors.New("o no")
					err := configurer.ConfigureHost(config)
					Expect(err).To(MatchError(&network.SetNsFailedError{linkConfigurer.SetNsReturns, vethCreator.CreateReturns.Container, 3}))
				})
			})

			Describe("adding the host to the bridge", func() {
				Context("when the bridge interface does not exist", func() {
					It("returns a wrapped error", func() {
						config.BridgeName = "bridge-that-doesnt-exist"
						config.BridgeIP = net.ParseIP("1.2.3.1")
						_, config.Subnet, _ = net.ParseCIDR("1.2.3.0/30")
						err := configurer.ConfigureHost(config)
						Expect(err).To(HaveOccurred())
					})
				})

				Context("when the bridge interface exists", func() {
					It("adds the host interface to the existing bridge", func() {
						config.BridgeName = "bridge"
						Expect(configurer.ConfigureHost(config)).To(Succeed())
						Expect(bridger.AddCalledWith.Bridge).To(Equal(existingBridge))
					})

					It("brings the host interface up", func() {
						config.BridgeName = "bridge"
						Expect(configurer.ConfigureHost(config)).To(Succeed())
						Expect(linkConfigurer.SetUpCalledWith).To(ContainElement(vethCreator.CreateReturns.Host))
					})

					Context("when bringing the host interface up fails", func() {
						It("returns a wrapped error", func() {
							cause := errors.New("there's jam in this sandwich and it's not ok")
							linkConfigurer.SetUpFunc = func(intf *net.Interface) error {
								if vethCreator.CreateReturns.Host == intf {
									return cause
								}

								return nil
							}

							config.BridgeName = "bridge"
							err := configurer.ConfigureHost(config)
							Expect(err).To(MatchError(&network.LinkUpError{cause, vethCreator.CreateReturns.Host, "host"}))
						})
					})
				})
			})

		})
	})

	Describe("ConfigureContainer", func() {
		var (
			linkConfigurer *fakedevices.FakeLink
			hostnameSetter *fakes.FakeHostname
			configurer     *network.NetworkConfigurer
			config         *network.ContainerConfig
		)

		BeforeEach(func() {
			linkConfigurer = &fakedevices.FakeLink{AddIPReturns: make(map[string]error)}
			hostnameSetter = &fakes.FakeHostname{}
			configurer = &network.NetworkConfigurer{
				Link:     linkConfigurer,
				Hostname: hostnameSetter,
			}
			config = &network.ContainerConfig{}
		})

		Context("when the loopback device does not exist", func() {
			var eth *net.Interface
			BeforeEach(func() {
				linkConfigurer.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					if name != "lo" {
						return eth, true, nil
					}

					return nil, false, nil
				}
			})

			It("returns a wrapped error", func() {
				err := configurer.ConfigureContainer(config)
				Expect(err).To(MatchError(&network.FindLinkError{nil, "loopback", "lo"}))
			})

			It("does not attempt to configure other devices", func() {
				Expect(configurer.ConfigureContainer(config)).ToNot(Succeed())
				Expect(linkConfigurer.SetUpCalledWith).ToNot(ContainElement(eth))
			})
		})

		Context("when the loopback exists", func() {
			var lo *net.Interface

			BeforeEach(func() {
				lo = &net.Interface{Name: "lo"}
				linkConfigurer.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					return &net.Interface{Name: name}, true, nil
				}
			})

			It("sets the hostname of the container", func() {
				err := configurer.ConfigureContainer(&network.ContainerConfig{
					Hostname: "somehost",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(hostnameSetter.SetHostnameCallCount()).To(Equal(1))
				Expect(hostnameSetter.SetHostnameArgsForCall(0)).To(Equal("somehost"))
			})

			Context("when setting the hostname returns an error", func() {
				BeforeEach(func() {
					hostnameSetter.SetHostnameReturns(errors.New("oh no!"))
				})

				It("returns the error", func() {
					err := configurer.ConfigureContainer(&network.ContainerConfig{
						Hostname: "somehost",
					})
					Expect(err).To(MatchError("oh no!"))
				})
			})

			It("adds 127.0.0.1/8 as an address", func() {
				ip, subnet, _ := net.ParseCIDR("127.0.0.1/8")
				Expect(configurer.ConfigureContainer(config)).To(Succeed())
				Expect(linkConfigurer.AddIPCalledWith).To(ContainElement(fakedevices.InterfaceIPAndSubnet{lo, ip, subnet}))
			})

			Context("when adding the IP address fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.AddIPReturns["lo"] = errors.New("o no")
					err := configurer.ConfigureContainer(config)
					ip, subnet, _ := net.ParseCIDR("127.0.0.1/8")
					Expect(err).To(MatchError(&network.ConfigureLinkError{errors.New("o no"), "loopback", lo, ip, subnet}))
				})
			})

			It("brings it up", func() {
				Expect(configurer.ConfigureContainer(config)).To(Succeed())
				Expect(linkConfigurer.SetUpCalledWith).To(ContainElement(lo))
			})

			Context("when bringing the link up fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.SetUpFunc = func(intf *net.Interface) error {
						return errors.New("o no")
					}

					err := configurer.ConfigureContainer(config)
					Expect(err).To(MatchError(&network.LinkUpError{errors.New("o no"), lo, "loopback"}))
				})
			})
		})

		Context("when the container interface does not exist", func() {
			BeforeEach(func() {
				linkConfigurer.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					if name == "lo" {
						return &net.Interface{Name: name}, true, nil
					}

					return nil, false, nil
				}
			})

			It("returns a wrapped error", func() {
				config.ContainerIntf = "foo"
				err := configurer.ConfigureContainer(config)
				Expect(err).To(MatchError(&network.FindLinkError{nil, "container", "foo"}))
			})
		})

		Context("when the container interface exists", func() {
			BeforeEach(func() {
				linkConfigurer.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					return &net.Interface{Name: name}, true, nil
				}
			})

			It("Adds the requested IP", func() {
				config.ContainerIntf = "foo"
				config.ContainerIP, config.Subnet, _ = net.ParseCIDR("2.3.4.5/6")

				Expect(configurer.ConfigureContainer(config)).To(Succeed())
				Expect(linkConfigurer.AddIPCalledWith).To(ContainElement(fakedevices.InterfaceIPAndSubnet{
					&net.Interface{Name: "foo"},
					config.ContainerIP,
					config.Subnet,
				}))
			})

			Context("when adding the IP fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.AddIPReturns["foo"] = errors.New("o no")

					config.ContainerIntf = "foo"
					config.ContainerIP, config.Subnet, _ = net.ParseCIDR("2.3.4.5/6")
					err := configurer.ConfigureContainer(config)
					Expect(err).To(MatchError(&network.ConfigureLinkError{
						errors.New("o no"),
						"container",
						&net.Interface{Name: "foo"},
						config.ContainerIP,
						config.Subnet,
					}))
				})
			})

			It("Brings the link up", func() {
				config.ContainerIntf = "foo"
				Expect(configurer.ConfigureContainer(config)).To(Succeed())
				Expect(linkConfigurer.SetUpCalledWith).To(ContainElement(&net.Interface{Name: "foo"}))
			})

			Context("when bringing the link up fails", func() {
				It("returns a wrapped error", func() {
					cause := errors.New("who ate my pie?")
					linkConfigurer.SetUpFunc = func(iface *net.Interface) error {
						if iface.Name == "foo" {
							return cause
						}

						return nil
					}

					config.ContainerIntf = "foo"
					err := configurer.ConfigureContainer(config)
					Expect(err).To(MatchError(&network.LinkUpError{cause, &net.Interface{Name: "foo"}, "container"}))
				})
			})

			It("sets the mtu", func() {
				config.ContainerIntf = "foo"
				config.Mtu = 1234
				Expect(configurer.ConfigureContainer(config)).To(Succeed())
				Expect(linkConfigurer.SetMTUCalledWith.Interface).To(Equal(&net.Interface{Name: "foo"}))
				Expect(linkConfigurer.SetMTUCalledWith.MTU).To(Equal(1234))
			})

			Context("when setting the mtu fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.SetMTUReturns = errors.New("this is NOT the right potato")

					config.ContainerIntf = "foo"
					config.Mtu = 1234
					err := configurer.ConfigureContainer(config)
					Expect(err).To(MatchError(&network.MTUError{linkConfigurer.SetMTUReturns, &net.Interface{Name: "foo"}, 1234}))
				})
			})

			It("adds a default gateway with the requested IP", func() {
				config.ContainerIntf = "foo"
				config.GatewayIP = net.ParseIP("2.3.4.5")
				Expect(configurer.ConfigureContainer(config)).To(Succeed())
				Expect(linkConfigurer.AddDefaultGWCalledWith.Interface).To(Equal(&net.Interface{Name: "foo"}))
				Expect(linkConfigurer.AddDefaultGWCalledWith.IP).To(Equal(net.ParseIP("2.3.4.5")))
			})

			Context("when adding a default gateway fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.AddDefaultGWReturns = errors.New("this is NOT the right potato")

					config.ContainerIntf = "foo"
					config.GatewayIP = net.ParseIP("2.3.4.5")
					err := configurer.ConfigureContainer(config)
					Expect(err).To(MatchError(&network.ConfigureDefaultGWError{linkConfigurer.AddDefaultGWReturns, &net.Interface{Name: "foo"}, net.ParseIP("2.3.4.5")}))
				})
			})
		})
	})
})
