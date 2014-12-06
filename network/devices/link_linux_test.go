package devices_test

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/network/devices"
	"github.com/docker/libcontainer/netlink"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Link Management", func() {
	var (
		l    devices.Link
		name string
		intf *net.Interface
	)

	BeforeEach(func() {
		name = fmt.Sprintf("gdn-test-%d", GinkgoParallelNode())
		Ω(netlink.NetworkLinkAdd(name, "dummy")).Should(Succeed())
		intf, _ = net.InterfaceByName(name)
	})

	AfterEach(func() {
		cleanup(name)
	})

	Describe("AddIP", func() {
		Context("when the interface exists", func() {
			It("adds the IP succesffuly", func() {
				ip, subnet, _ := net.ParseCIDR("1.2.3.4/5")
				Ω(l.AddIP(intf, ip, subnet)).Should(Succeed())

				intf, err := net.InterfaceByName(name)
				Ω(err).ShouldNot(HaveOccurred())
				addrs, err := intf.Addrs()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(addrs).Should(HaveLen(1))
				Ω(addrs[0].String()).Should(Equal("1.2.3.4/5"))
			})
		})
	})

	Describe("SetUp", func() {
		Context("when the interface does not exist", func() {
			It("returns an error", func() {
				Ω(l.SetUp(&net.Interface{Name: "something"})).ShouldNot(Succeed())
			})
		})

		Context("when the interface exists", func() {
			Context("and it is down", func() {
				It("should bring the interface up", func() {
					Ω(l.SetUp(intf)).Should(Succeed())

					intf, err := net.InterfaceByName(name)
					Ω(err).ShouldNot(HaveOccurred())
					Ω(intf.Flags & net.FlagUp).Should(Equal(net.FlagUp))
				})
			})

			Context("and it is already up", func() {
				It("should still succeed", func() {
					Ω(l.SetUp(intf)).Should(Succeed())
					Ω(l.SetUp(intf)).Should(Succeed())

					intf, err := net.InterfaceByName(name)
					Ω(err).ShouldNot(HaveOccurred())
					Ω(intf.Flags & net.FlagUp).Should(Equal(net.FlagUp))
				})
			})
		})
	})

	Describe("SetMTU", func() {
		Context("when the interface does not exist", func() {
			It("returns an error", func() {
				Ω(l.SetMTU(&net.Interface{Name: "something"}, 1234)).ShouldNot(Succeed())
			})
		})

		Context("when the interface exists", func() {
			It("sets the mtu", func() {
				Ω(l.SetMTU(intf, 1234)).Should(Succeed())

				intf, err := net.InterfaceByName(name)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(intf.MTU).Should(Equal(1234))
			})
		})
	})

	Describe("SetNs", func() {
		BeforeEach(func() {
			cmd, err := gexec.Start(exec.Command("sh", "-c", "mount -n -t tmpfs tmpfs /sys; ip netns add gdnsetnstest"), GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())
			Eventually(cmd).Should(gexec.Exit(0))
		})

		AfterEach(func() {
			cmd, err := gexec.Start(exec.Command("sh", "-c", "ip netns delete gdnsetnstest; umount /sys"), GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())
			Eventually(cmd).Should(gexec.Exit(0))
		})

		It("moves the interface in to the given namespace by pid", func() {
			// look at this perfectly ordinary hat
			netns, err := gexec.Start(exec.Command("ip", "netns", "exec", "gdnsetnstest", "sleep", "6312736"), GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())
			defer netns.Kill()

			// (it has the following pid)
			ps, err := gexec.Start(exec.Command("sh", "-c", "ps -A -opid,command | grep 'sleep 6312736' | head -n 1 | awk '{print $1}'"), GinkgoWriter, GinkgoWriter) // look at my hat
			Ω(err).ShouldNot(HaveOccurred())
			Eventually(ps).Should(gexec.Exit(0))
			pid, err := strconv.Atoi(strings.TrimSuffix(string(ps.Out.Contents()), "\n"))
			Ω(err).ShouldNot(HaveOccurred())

			// I wave the magic wand
			Ω(l.SetNs(intf, pid)).Should(Succeed())

			// the bunny has vanished! where is the bunny?
			intfs, _ := net.Interfaces()
			Ω(intfs).ShouldNot(ContainElement(intf))

			// oh my word it's in the hat!
			session, err := gexec.Start(exec.Command("sh", "-c", fmt.Sprintf("ip netns exec gdnsetnstest ifconfig %s", name)), GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())
			Eventually(session).Should(gexec.Exit(0))

		})
	})

	Describe("InterfaceByName", func() {
		Context("when the interface exists", func() {
			It("returns the interface with the given name, and true", func() {
				returnedIntf, found, err := l.InterfaceByName(name)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(returnedIntf).Should(Equal(intf))
				Ω(found).Should(BeTrue())
			})
		})

		Context("when the interface does not exist", func() {
			It("does not return an error", func() {
				_, found, err := l.InterfaceByName("sandwich")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(found).Should(BeFalse())
			})
		})
	})
})
