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
				stubInterfaceByNameError error
				stubInterfaceByNameName  string
				stubInterfaceByNameIfc   *net.Interface = &net.Interface{}

				stubNetworkSetMTUError error
				stubNetworkSetMTUIfc   *net.Interface
				stubNetworkSetMTUMtu   int

				stubNetworkLinkAddIpError       error
				stubNetworkLinkAddIpIfc         *net.Interface
				stubNetworkLinkAddIpContainerIP net.IP
				stubNetworkLinkAddIpsubnet      *net.IPNet

				stubAddDefaultGwError   error
				stubAddDefaultGwIP      string
				stubAddDefaultGwIfcName string

				stubNetworkLinkUpError error
				stubNetworkLinkUpIfc   *net.Interface
			)

			BeforeEach(func() {
				stubInterfaceByNameError = nil
				stubInterfaceByNameName = ""
				stubInterfaceByNameIfc = nil
				network.InterfaceByName = func(name string) (*net.Interface, error) {
					stubInterfaceByNameName = name
					if stubInterfaceByNameError != nil {
						return nil, stubInterfaceByNameError
					}
					return stubInterfaceByNameIfc, nil
				}

				stubNetworkSetMTUError = nil
				stubNetworkSetMTUIfc = nil
				stubNetworkSetMTUMtu = 0
				network.NetworkSetMTU = func(iface *net.Interface, mtu int) error {
					stubNetworkSetMTUIfc = iface
					stubNetworkSetMTUMtu = mtu
					if stubNetworkSetMTUError != nil {
						return stubNetworkSetMTUError
					}
					return nil
				}

				stubNetworkLinkAddIpError = nil
				stubNetworkLinkAddIpIfc = nil
				stubNetworkLinkAddIpContainerIP = net.ParseIP("0.0.0.0")
				stubNetworkLinkAddIpsubnet = nil
				network.NetworkLinkAddIp = func(iface *net.Interface, ip net.IP, ipNet *net.IPNet) error {
					stubNetworkLinkAddIpIfc = iface
					stubNetworkLinkAddIpContainerIP = ip
					stubNetworkLinkAddIpsubnet = ipNet
					if stubNetworkLinkAddIpError != nil {
						return stubNetworkLinkAddIpError
					}
					return nil
				}

				stubAddDefaultGwError = nil
				stubAddDefaultGwIP = ""
				stubAddDefaultGwIfcName = ""
				network.AddDefaultGw = func(ip, device string) error {
					stubAddDefaultGwIP = ip
					stubAddDefaultGwIfcName = device
					if stubAddDefaultGwError != nil {
						return stubAddDefaultGwError
					}
					return nil
				}

				stubNetworkLinkUpError = nil
				stubNetworkLinkUpIfc = nil
				network.NetworkLinkUp = func(iface *net.Interface) error {
					stubNetworkLinkUpIfc = iface
					if stubNetworkLinkUpError != nil {
						return stubNetworkLinkUpError
					}
					return nil
				}
			})

			It("passes the correct parameters to InterfaceByName", func() {
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(stubInterfaceByNameName).Should(Equal(containerInterfaceName))
			})

			It("returns an error when InterfaceByName fails", func() {
				stubInterfaceByNameError = testError
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
				stubNetworkSetMTUError = testError
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

			It("returns an error when NetworkLinkAddIp fails", func() {
				stubNetworkLinkAddIpError = testError
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
				stubAddDefaultGwError = testError
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrFailedToAddGateway))
			})

			It("passes the correct parameters to NetworkLinkUp", func() {
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(stubNetworkLinkUpIfc).Should(Equal(stubInterfaceByNameIfc))
			})

			It("returns an error when NetworkLinkUp fails", func() {
				stubNetworkLinkUpError = testError
				err := network.ConfigureContainer(containerInterfaceName, containerIP, gatewayIP, subnet, mtu)
				Ω(err).Should(Equal(network.ErrFailedToLinkUp))
			})
		})

	})
})
