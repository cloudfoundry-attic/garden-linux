package network_test

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/devices/fakedevices"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var testError = errors.New("test error")

var _ = Describe("Configure", func() {
	Describe("ConfigureHost", func() {
		var (
			vethCreator    *fakedevices.FaveVethCreator
			linkConfigurer *fakedevices.FakeLink
			bridger        *fakedevices.FakeBridge

			configurer     *network.Configurer
			existingBridge *net.Interface
		)

		BeforeEach(func() {
			vethCreator = &fakedevices.FaveVethCreator{}
			linkConfigurer = &fakedevices.FakeLink{AddIPReturns: make(map[string]error)}
			bridger = &fakedevices.FakeBridge{}
			configurer = &network.Configurer{Veth: vethCreator, Link: linkConfigurer, Bridge: bridger, Logger: lagertest.NewTestLogger("test")}

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

		It("creates a virtual ethernet pair", func() {
			Ω(configurer.ConfigureHost("host", "container", "bridge", 0, nil, nil, 0)).Should(Succeed())

			Ω(vethCreator.CreateCalledWith.HostIfcName).Should(Equal("host"))
			Ω(vethCreator.CreateCalledWith.ContainerIfcName).Should(Equal("container"))
		})

		Context("when creating the pair fails", func() {
			It("returns a wrapped error", func() {
				vethCreator.CreateReturns.Err = errors.New("foo bar baz")
				err := configurer.ConfigureHost("host", "container", "bridge", 0, nil, nil, 0)
				Ω(err).Should(HaveOccurred())
				Ω(err).Should(MatchError(&network.VethPairCreationError{vethCreator.CreateReturns.Err, "host", "container"}))
			})
		})

		Context("when creating the pair succeeds", func() {
			BeforeEach(func() {
				vethCreator.CreateReturns.Host = &net.Interface{Name: "the-host"}
				vethCreator.CreateReturns.Container = &net.Interface{Name: "the-container"}
			})

			It("should set mtu on the host interface", func() {
				Ω(configurer.ConfigureHost("host", "", "bridge", 0, nil, nil, 123)).Should(Succeed())

				Ω(linkConfigurer.SetMTUCalledWith.Interface).Should(Equal(vethCreator.CreateReturns.Host))
				Ω(linkConfigurer.SetMTUCalledWith.MTU).Should(Equal(123))
			})

			Context("When setting the mtu fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.SetMTUReturns = errors.New("o no")
					err := configurer.ConfigureHost("host", "container", "bridge", 0, nil, nil, 14)
					Ω(err).Should(MatchError(&network.MTUError{linkConfigurer.SetMTUReturns, vethCreator.CreateReturns.Host, 14}))
				})
			})

			It("should move the container interface in to the container's namespace", func() {
				Ω(configurer.ConfigureHost("", "", "bridge", 3, nil, nil, 0)).Should(Succeed())
				Ω(linkConfigurer.SetNsCalledWith.Pid).Should(Equal(3))
			})

			Context("When moving the container interface into the namespace fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.SetNsReturns = errors.New("o no")
					err := configurer.ConfigureHost("", "", "bridge", 3, nil, nil, 0)
					Ω(err).Should(MatchError(&network.SetNsFailedError{linkConfigurer.SetNsReturns, vethCreator.CreateReturns.Container, 3}))
				})
			})

			Describe("adding the host to the bridge", func() {
				Context("when the bridge interface does not exist", func() {
					It("returns a wrapped error", func() {
						_, subnet, _ := net.ParseCIDR("1.2.3.0/30")
						err := configurer.ConfigureHost("", "", "bridge-that-doesnt-exist", 0, net.ParseIP("1.2.3.1"), subnet, 0)
						Ω(err).Should(HaveOccurred())
					})
				})

				Context("when the bridge interface exists", func() {
					It("adds the host interface to the existing bridge", func() {
						Ω(configurer.ConfigureHost("", "", "bridge", 0, nil, nil, 0)).Should(Succeed())
						Ω(bridger.AddCalledWith.Bridge).Should(Equal(existingBridge))
					})

					It("brings the host interface up", func() {
						Ω(configurer.ConfigureHost("", "", "bridge", 0, nil, nil, 0)).Should(Succeed())
						Ω(linkConfigurer.SetUpCalledWith).Should(ContainElement(vethCreator.CreateReturns.Host))
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

							err := configurer.ConfigureHost("", "", "bridge", 0, nil, nil, 0)
							Ω(err).Should(MatchError(&network.LinkUpError{cause, vethCreator.CreateReturns.Host, "host"}))
						})
					})
				})
			})

		})
	})

	Describe("ConfigureContainer", func() {
		var (
			linkConfigurer *fakedevices.FakeLink
			configurer     *network.Configurer
		)

		BeforeEach(func() {
			linkConfigurer = &fakedevices.FakeLink{AddIPReturns: make(map[string]error)}
			configurer = &network.Configurer{Link: linkConfigurer}
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
				err := configurer.ConfigureContainer("", nil, nil, nil, 0)
				Ω(err).Should(MatchError(&network.FindLinkError{nil, "loopback", "lo"}))
			})

			It("does not attempt to configure other devices", func() {
				Ω(configurer.ConfigureContainer("", nil, nil, nil, 0)).ShouldNot(Succeed())
				Ω(linkConfigurer.SetUpCalledWith).ShouldNot(ContainElement(eth))
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

			It("adds 127.0.0.1/8 as an address", func() {
				ip, subnet, _ := net.ParseCIDR("127.0.0.1/8")
				Ω(configurer.ConfigureContainer("", nil, nil, nil, 0)).Should(Succeed())
				Ω(linkConfigurer.AddIPCalledWith).Should(ContainElement(fakedevices.InterfaceIPAndSubnet{lo, ip, subnet}))
			})

			Context("when adding the IP address fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.AddIPReturns["lo"] = errors.New("o no")
					err := configurer.ConfigureContainer("", nil, nil, nil, 0)
					ip, subnet, _ := net.ParseCIDR("127.0.0.1/8")
					Ω(err).Should(MatchError(&network.ConfigureLinkError{errors.New("o no"), "loopback", lo, ip, subnet}))
				})
			})

			It("brings it up", func() {
				Ω(configurer.ConfigureContainer("", nil, nil, nil, 0)).Should(Succeed())
				Ω(linkConfigurer.SetUpCalledWith).Should(ContainElement(lo))
			})

			Context("when bringing the link up fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.SetUpFunc = func(intf *net.Interface) error {
						return errors.New("o no")
					}

					err := configurer.ConfigureContainer("", nil, nil, nil, 0)
					Ω(err).Should(MatchError(&network.LinkUpError{errors.New("o no"), lo, "loopback"}))
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
				err := configurer.ConfigureContainer("foo", nil, nil, nil, 0)
				Ω(err).Should(MatchError(&network.FindLinkError{nil, "container", "foo"}))
			})
		})

		Context("when the container interface exists", func() {
			BeforeEach(func() {
				linkConfigurer.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					return &net.Interface{Name: name}, true, nil
				}
			})

			It("Adds the requested IP", func() {
				ip, subnet, _ := net.ParseCIDR("2.3.4.5/6")
				Ω(configurer.ConfigureContainer("foo", ip, nil, subnet, 0)).Should(Succeed())
				Ω(linkConfigurer.AddIPCalledWith).Should(ContainElement(fakedevices.InterfaceIPAndSubnet{&net.Interface{Name: "foo"}, ip, subnet}))
			})

			Context("when adding the IP fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.AddIPReturns["foo"] = errors.New("o no")

					ip, subnet, _ := net.ParseCIDR("2.3.4.5/6")
					err := configurer.ConfigureContainer("foo", ip, nil, subnet, 0)
					Ω(err).Should(MatchError(&network.ConfigureLinkError{errors.New("o no"), "container", &net.Interface{Name: "foo"}, ip, subnet}))
				})
			})

			It("Brings the link up", func() {
				Ω(configurer.ConfigureContainer("foo", nil, nil, nil, 0)).Should(Succeed())
				Ω(linkConfigurer.SetUpCalledWith).Should(ContainElement(&net.Interface{Name: "foo"}))
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

					err := configurer.ConfigureContainer("foo", nil, nil, nil, 0)
					Ω(err).Should(MatchError(&network.LinkUpError{cause, &net.Interface{Name: "foo"}, "container"}))
				})
			})

			It("sets the mtu", func() {
				Ω(configurer.ConfigureContainer("foo", nil, nil, nil, 1234)).Should(Succeed())
				Ω(linkConfigurer.SetMTUCalledWith.Interface).Should(Equal(&net.Interface{Name: "foo"}))
				Ω(linkConfigurer.SetMTUCalledWith.MTU).Should(Equal(1234))
			})

			Context("when setting the mtu fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.SetMTUReturns = errors.New("this is NOT the right potato")

					err := configurer.ConfigureContainer("foo", nil, nil, nil, 1234)
					Ω(err).Should(MatchError(&network.MTUError{linkConfigurer.SetMTUReturns, &net.Interface{Name: "foo"}, 1234}))
				})
			})

			It("adds a default gateway with the requested IP", func() {
				Ω(configurer.ConfigureContainer("foo", nil, net.ParseIP("2.3.4.5"), nil, 0)).Should(Succeed())
				Ω(linkConfigurer.AddDefaultGWCalledWith.Interface).Should(Equal(&net.Interface{Name: "foo"}))
				Ω(linkConfigurer.AddDefaultGWCalledWith.IP).Should(Equal(net.ParseIP("2.3.4.5")))
			})

			Context("when adding a default gateway fails", func() {
				It("returns a wrapped error", func() {
					linkConfigurer.AddDefaultGWReturns = errors.New("this is NOT the right potato")

					err := configurer.ConfigureContainer("foo", nil, net.ParseIP("2.3.4.5"), nil, 0)
					Ω(err).Should(MatchError(&network.ConfigureDefaultGWError{linkConfigurer.AddDefaultGWReturns, &net.Interface{Name: "foo"}, net.ParseIP("2.3.4.5")}))
				})
			})
		})
	})
})
