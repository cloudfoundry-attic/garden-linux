package networking_test

import (
	"fmt"
	"io"
	"strings"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Net Out", func() {
	var (
		container      garden.Container
		otherContainer garden.Container
		gardenArgs     []string

		containerNetwork string
		denyRange        string
		allowRange       string
	)

	const containerHandle = "6e4ea858-6b31-4243-5dcc-093cfb83952d"

	BeforeEach(func() {
		denyRange = ""
		allowRange = ""
		gardenArgs = []string{}
	})

	JustBeforeEach(func() {
		gardenArgs = []string{
			"-denyNetworks", strings.Join([]string{
				denyRange,
				allowRange, // so that it can be overridden by allowNetworks below
			}, ","),
			"-allowNetworks", allowRange,
			"-iptablesLogMethod", "nflog", // so that we can read logs when running in fly
		}
		client = startGarden(gardenArgs...)

		var err error
		container, err = client.Create(garden.ContainerSpec{Network: containerNetwork, Privileged: true, Handle: containerHandle})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())
	})

	runInContainer := func(container garden.Container, script string) (garden.Process, *gbytes.Buffer) {
		out := gbytes.NewBuffer()
		process, err := container.Run(garden.ProcessSpec{
			Path: "sh",
			Args: []string{"-c", script},
		}, garden.ProcessIO{
			Stdout: io.MultiWriter(out, GinkgoWriter),
			Stderr: GinkgoWriter,
		})
		Ω(err).ShouldNot(HaveOccurred())

		return process, out
	}

	Context("external addresses", func() {
		var (
			ByAllowingTCP, ByRejectingTCP func()
		)

		BeforeEach(func() {
			ByAllowingTCP = func() {
				By("allowing outbound tcp traffic", func() {
					Ω(checkInternet(container)).Should(Succeed())
				})
			}

			ByRejectingTCP = func() {
				By("rejecting outbound tcp traffic", func() {
					Ω(checkInternet(container)).Should(HaveOccurred())
				})
			}
		})

		Context("when the target address is inside DENY_NETWORKS", func() {
			//The target address is the ip addr of www.example.com in these tests
			BeforeEach(func() {
				denyRange = "0.0.0.0/0"
				allowRange = "9.9.9.9/30"
				containerNetwork = fmt.Sprintf("10.1%d.0.0/24", GinkgoParallelNode())
			})

			It("disallows TCP connections", func() {
				ByRejectingTCP()
			})

			Context("when a rule that allows all traffic to the target is added", func() {
				JustBeforeEach(func() {
					err := container.NetOut(garden.NetOutRule{
						Networks: []garden.IPRange{
							garden.IPRangeFromIP(externalIP),
						},
					})
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("allows TCP traffic to the target", func() {
					ByAllowingTCP()
				})
			})
		})

		Context("when the target address is inside ALLOW_NETWORKS", func() {
			BeforeEach(func() {
				denyRange = "0.0.0.0/0"
				allowRange = "0.0.0.0/0"
				containerNetwork = fmt.Sprintf("10.1%d.0.0/24", GinkgoParallelNode())
			})

			It("allows connections", func() {
				ByAllowingTCP()
			})
		})

		Context("when the target address is in neither ALLOW_NETWORKS nor DENY_NETWORKS", func() {
			BeforeEach(func() {
				denyRange = "4.4.4.4/30"
				allowRange = "4.4.4.4/30"
				containerNetwork = fmt.Sprintf("10.1%d.0.0/24", GinkgoParallelNode())
			})

			It("allows connections", func() {
				ByAllowingTCP()
			})
		})

		Context("when there are two containers in the same subnet", func() {
			BeforeEach(func() {
				denyRange = "0.0.0.0/0"
				containerNetwork = fmt.Sprintf("10.1%d.0.0/24", GinkgoParallelNode())
			})

			It("does not allow rules from the second container to affect the first", func() {
				var err error
				secondContainer, err := client.Create(garden.ContainerSpec{Network: containerNetwork, Privileged: true})
				Ω(err).ShouldNot(HaveOccurred())

				ByRejectingTCP()

				Ω(secondContainer.NetOut(garden.NetOutRule{
					Networks: []garden.IPRange{
						garden.IPRangeFromIP(externalIP),
					},
				})).Should(Succeed())

				By("continuing to reject")
				ByRejectingTCP()
			})
		})
	})

	Describe("Other Containers", func() {

		const tcpPort = 8080

		targetIP := func(c garden.Container) string {
			info, err := c.Info()
			Ω(err).ShouldNot(HaveOccurred())
			return info.ContainerIP
		}

		ByAllowingTCP := func() {
			By("allowing tcp traffic to it", func() {
				Ω(checkConnection(container, targetIP(otherContainer), tcpPort)).Should(Succeed())
			})
		}

		Context("containers in the same subnet", func() {
			JustBeforeEach(func() {
				var err error
				otherContainer, err = client.Create(garden.ContainerSpec{Network: containerNetwork})
				Ω(err).ShouldNot(HaveOccurred())

				runInContainer(otherContainer, fmt.Sprintf("echo hello | nc -l -p %d", tcpPort)) //tcp
			})

			Context("even if the address is in deny networks", func() {
				BeforeEach(func() {
					denyRange = "0.0.0.0/8"
					allowRange = ""
					containerNetwork = fmt.Sprintf("10.1%d.0.0/24", GinkgoParallelNode())
				})

				It("can route to each other", func() {
					ByAllowingTCP()
				})
			})
		})

		Context("containers in distinct subnets", func() {
			var otherContainerNetwork string

			JustBeforeEach(func() {
				otherContainerNetwork = fmt.Sprintf("10.1%d.1.0/24", GinkgoParallelNode())
				var err error
				otherContainer, err = client.Create(garden.ContainerSpec{Network: otherContainerNetwork})
				Ω(err).ShouldNot(HaveOccurred())

				runInContainer(otherContainer, fmt.Sprintf("echo hello | nc -l -p %d", tcpPort)) //tcp
			})

			Context("when deny networks is empty", func() {
				It("can route to each other", func() {
					ByAllowingTCP()
				})
			})
		})
	})
})
