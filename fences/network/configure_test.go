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
