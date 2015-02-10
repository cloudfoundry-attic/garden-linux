package networking_test

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	archiver "github.com/pivotal-golang/archiver/extractor/test_helper"
)

var _ = Describe("Net In/Out", func() {
	var (
		container      garden.Container
		otherContainer garden.Container

		containerNetwork string
		denyRange        string
		allowRange       string
	)

	BeforeEach(func() {
		denyRange = ""
		allowRange = ""
	})

	JustBeforeEach(func() {
		client = startGarden(
			"-denyNetworks", strings.Join([]string{
				denyRange,
				allowRange, // so that it can be overridden by allowNetworks below
			}, ","),
			"-allowNetworks", allowRange,
			"-iptablesLogMethod", "nflog", // so that we can read logs when running in fly
		)

		var err error
		container, err = client.Create(garden.ContainerSpec{Network: containerNetwork, Privileged: true})
		Ω(err).ShouldNot(HaveOccurred())

		Ω(container.StreamIn("bin/", tgzReader(netdogBin))).Should(Succeed())
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
			ByAllowingTCP, ByAllowingICMPPings, ByRejectingTCP, ByRejectingICMPPings func()
		)

		BeforeEach(func() {
			ByAllowingTCP = func() {
				By("allowing outbound tcp traffic", func() {
					process, _ := runInContainer(
						container,
						fmt.Sprintf("(echo 'GET / HTTP/1.1'; echo 'Host: example.com'; echo) | nc -w5 %s 80", externalIP),
					)

					Ω(process.Wait()).Should(Equal(0))
				})
			}

			ByAllowingICMPPings = func() {
				if err := exec.Command("sh", "-c", fmt.Sprintf("ping -c 1 -w 1 %s", externalIP)).Run(); err != nil {
					fmt.Println("Ginkgo host environment cannot ping out, skipping ICMP out test: ", err)
					return
				}

				By("allowing outbound icmp traffic", func() {
					// sacrificial ping, which appears not to work on first packet
					runInContainer(
						container,
						fmt.Sprintf("ping -c 1 %s", externalIP),
					)
					_, out := runInContainer(
						container,
						fmt.Sprintf("ping -c 1 -w 1 %s", externalIP),
					)

					Eventually(out, "5s").Should(gbytes.Say(" 0% packet loss"), "lost packets on ping")
				})
			}

			ByRejectingTCP = func() {
				By("rejecting outbound tcp traffic", func() {
					process, _ := runInContainer(
						container,
						fmt.Sprintf("(echo 'GET / HTTP/1.1'; echo 'Host: example.com'; echo) | nc -w5 %s 80", externalIP),
					)

					Ω(process.Wait()).Should(Equal(1))
				})
			}

			ByRejectingICMPPings = func() {
				if err := exec.Command("sh", "-c", fmt.Sprintf("ping  -c 1 -w 1 %s", externalIP)).Run(); err != nil {
					fmt.Println("Ginkgo host environment cannot ping out, skipping ICMP out test: ", err)
					return
				}

				By("rejecting outbound icmp traffic", func() {
					// sacrificial ping, which appears not to work on first packet anyway
					runInContainer(
						container,
						fmt.Sprintf("ping -c 1 %s", externalIP),
					)
					_, out := runInContainer(
						container,
						fmt.Sprintf("ping -c 1 -w 1 %s", externalIP),
					)

					Eventually(out, "5s").Should(gbytes.Say(" 100% packet loss"))
				})
			}
		})

		Context("when the target address is inside DENY_NETWORKS", func() {
			BeforeEach(func() {
				denyRange = "0.0.0.0/0"
				allowRange = "9.9.9.9/30"
				containerNetwork = fmt.Sprintf("10.1%d.0.0/24", GinkgoParallelNode())
			})

			Context("by default", func() {
				It("disallows connections", func() {
					ByRejectingTCP()
					ByRejectingICMPPings()
				})
			})

			Context("after a net_out of another range", func() {
				It("does not allow connections to that address", func() {
					container.NetOut(garden.NetOutRule{
						Protocol: garden.ProtocolAll,
						Networks: []garden.IPRange{
							IPRangeFromCIDR("1.2.3.4/30"),
						},
						Ports: []garden.PortRange{
							garden.PortRangeFromPort(0),
						},
					})
					ByRejectingTCP()
					ByRejectingICMPPings()
				})
			})

			Context("after net_out allows tcp traffic to that IP and port", func() {
				Context("when no port is specified", func() {
					It("allows both tcp and icmp to that address", func() {
						err := container.NetOut(garden.NetOutRule{
							Networks: []garden.IPRange{
								garden.IPRangeFromIP(externalIP),
							},
						})
						Ω(err).ShouldNot(HaveOccurred())
						ByAllowingTCP()
						ByAllowingICMPPings()
					})
				})

				Context("and the IP address is the first element of a list of allowed ranges", func() {
					It("allows both tcp and icmp to that address", func() {
						err := container.NetOut(garden.NetOutRule{
							Networks: []garden.IPRange{
								garden.IPRangeFromIP(externalIP),
								garden.IPRangeFromIP(net.ParseIP("9.9.9.9")),
							},
						})
						Ω(err).ShouldNot(HaveOccurred())
						ByAllowingTCP()
						ByAllowingICMPPings()
					})
				})

				Context("and the IP address is the final element of a list of allowed ranges", func() {
					It("allows both tcp and icmp to that address", func() {
						err := container.NetOut(garden.NetOutRule{
							Networks: []garden.IPRange{
								garden.IPRangeFromIP(net.ParseIP("9.9.9.9")),
								garden.IPRangeFromIP(externalIP),
							},
						})
						Ω(err).ShouldNot(HaveOccurred())
						ByAllowingTCP()
						ByAllowingICMPPings()
					})
				})
			})

			Context("after net_out allows tcp traffic to a range of IP addresses", func() {
				It("allows tcp to an address in the range", func() {
					err := container.NetOut(garden.NetOutRule{
						Networks: []garden.IPRange{
							garden.IPRange{Start: externalIP, End: net.ParseIP("255.255.255.254")},
						},
					})
					Ω(err).ShouldNot(HaveOccurred())
					ByAllowingTCP()
				})
			})

			Describe("allowing individual protocols", func() {
				// To prevent test pollution due to connection tracking, each test
				// should use a distinct container IP address.
				Context("when all TCP traffic is allowed", func() {
					BeforeEach(func() {
						containerNetwork = fmt.Sprintf("10.1%d.2.0/24", GinkgoParallelNode())
					})

					It("allows TCP and rejects ICMP pings", func() {
						err := container.NetOut(garden.NetOutRule{
							Protocol: garden.ProtocolTCP,
							Networks: []garden.IPRange{
								garden.IPRangeFromIP(externalIP),
							},
						})
						Ω(err).ShouldNot(HaveOccurred())
						ByAllowingTCP()
						ByRejectingICMPPings()
					})
				})

				Context("when ICMP non-ping type traffic is allowed", func() {
					BeforeEach(func() {
						containerNetwork = fmt.Sprintf("10.1%d.3.0/24", GinkgoParallelNode())
					})

					It("rejects ICMP pings and TCP", func() {
						err := container.NetOut(garden.NetOutRule{
							Protocol: garden.ProtocolICMP,
							Networks: []garden.IPRange{
								garden.IPRangeFromIP(externalIP),
							},
							ICMPs: &garden.ICMPControl{
								Type: 13,
								Code: garden.ICMPControlCode(0),
							},
						})
						Ω(err).ShouldNot(HaveOccurred())
						ByRejectingICMPPings()
						ByRejectingTCP()
					})
				})

				Context("when ICMP ping type/code traffic is allowed", func() {
					BeforeEach(func() {
						containerNetwork = fmt.Sprintf("10.1%d.4.0/24", GinkgoParallelNode())
					})

					It("allows ICMP pings and rejects TCP", func() {
						Ω(container.NetOut(garden.NetOutRule{
							Protocol: garden.ProtocolICMP,
							Networks: []garden.IPRange{
								garden.IPRangeFromIP(net.ParseIP(externalIP.String())),
							},
							ICMPs: &garden.ICMPControl{
								Type: 8,                         // ping request
								Code: garden.ICMPControlCode(4), // but not correct code
							},
						})).Should(Succeed())
						ByRejectingICMPPings()
						ByRejectingTCP()

						Ω(container.NetOut(garden.NetOutRule{
							Protocol: garden.ProtocolICMP,
							Networks: []garden.IPRange{
								garden.IPRangeFromIP(externalIP),
							},
							ICMPs: &garden.ICMPControl{
								Type: 8, // ping request, all codes accepted
							},
						})).Should(Succeed())
						ByAllowingICMPPings()
						ByRejectingTCP()
					})
				})

				Context("when all ICMP traffic is allowed", func() {
					BeforeEach(func() {
						containerNetwork = fmt.Sprintf("10.1%d.5.0/24", GinkgoParallelNode())
					})

					It("allows ICMP and rejects TCP", func() {
						Ω(container.NetOut(garden.NetOutRule{
							Protocol: garden.ProtocolICMP,
							Networks: []garden.IPRange{
								garden.IPRangeFromIP(externalIP),
							},
						})).Should(Succeed())
						ByAllowingICMPPings()
						ByRejectingTCP()
					})
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
				ByAllowingICMPPings()
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
				ByAllowingICMPPings()
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
		var (
			udpListenerOut *gbytes.Buffer
		)

		const tcpPort = 8080
		const udpPort = 8081
		const tcpPortRange = "8080:8090"
		const udpPortRange = "8081:8091"

		targetIP := func(c garden.Container) string {
			info, err := c.Info()
			Ω(err).ShouldNot(HaveOccurred())
			return info.ContainerIP
		}

		ByAllowingTCP := func(data ...string) {
			By("allowing tcp traffic to it", func() {
				msg := "hello"
				if len(data) > 0 {
					msg = data[0]
				}

				process, _ := runInContainer(
					container,
					fmt.Sprintf("echo "+msg+" | nc -w 1 %s %d", targetIP(otherContainer), tcpPort),
				)

				Ω(process.Wait()).Should(Equal(0))
			})
		}

		ByAllowingUDP := func() {
			By("allowing udp traffic to it", func() {
				process, _ := runInContainer(
					container,
					fmt.Sprintf("echo ok | ~/bin/netdog send %s:%d", targetIP(otherContainer), udpPort),
				)

				Ω(process.Wait()).Should(Equal(0))
				Eventually(udpListenerOut).Should(gbytes.Say("ok"))
			})
		}

		ByAllowingICMPPings := func() {
			By("allowing icmp traffic to it", func() {
				_, out := runInContainer(
					container,
					fmt.Sprintf("ping  -c 1 -w 1 %s", targetIP(otherContainer)),
				)

				Eventually(out, "5s").Should(gbytes.Say(" 0% packet loss"))
			})
		}

		ByRejectingTCP := func() {
			By("not allowing tcp traffic to it", func() {
				process, _ := runInContainer(
					container,
					fmt.Sprintf("echo hello | nc -w 4 %s %d", targetIP(otherContainer), tcpPort),
				)

				Ω(process.Wait()).Should(Equal(1))
			})
		}

		ByRejectingUDP := func() {
			By("not allowing udp traffic to it", func() {
				process, _ := runInContainer(
					container,
					fmt.Sprintf("echo ok | ~/bin/netdog send %s:%d", targetIP(otherContainer), udpPort),
				)

				Ω(process.Wait()).Should(Equal(0)) // udp is connectionless, we can send, it just shouldn't be received
				Consistently(udpListenerOut, "2s", "500ms").ShouldNot(gbytes.Say("ok"))
			})
		}

		ByRejectingICMPPings := func() {
			By("not allowing icmp traffic to it", func() {
				_, out := runInContainer(
					container,
					fmt.Sprintf("ping -c 1 -w 1 %s", targetIP(otherContainer)),
				)

				Eventually(out, "5s").Should(gbytes.Say(" 100% packet loss"))
			})
		}

		Context("containers in the same subnet", func() {
			JustBeforeEach(func() {
				var err error
				otherContainer, err = client.Create(garden.ContainerSpec{Network: containerNetwork})
				Ω(err).ShouldNot(HaveOccurred())

				runInContainer(otherContainer, fmt.Sprintf("echo hello | nc -l -p %d", tcpPort)) //tcp

				Ω(otherContainer.StreamIn("./bin/", tgzReader(netdogBin))).Should(Succeed())
				_, udpListenerOut = runInContainer(otherContainer, fmt.Sprintf("echo hello | ~/bin/netdog listen %d", udpPort)) //udp
				Eventually(udpListenerOut).Should(gbytes.Say("listening"))
			})

			Context("even if the address is in deny networks", func() {
				BeforeEach(func() {
					denyRange = "0.0.0.0/8"
					allowRange = ""
					containerNetwork = fmt.Sprintf("10.1%d.0.0/24", GinkgoParallelNode())
				})

				It("allows connections", func() {
					ByAllowingICMPPings()
					ByAllowingTCP()
					ByAllowingUDP()
				})
			})
		})

		Context("containers in other subnets", func() {
			var (
				otherContainerNetwork *net.IPNet
			)

			BeforeEach(func() {
				_, otherContainerNetwork, _ = net.ParseCIDR(fmt.Sprintf("10.2%d.0.1/24", GinkgoParallelNode()))
			})

			JustBeforeEach(func() {
				var err error
				otherContainer, err = client.Create(garden.ContainerSpec{Network: otherContainerNetwork.String()})
				Ω(err).ShouldNot(HaveOccurred())
				runInContainer(otherContainer, fmt.Sprintf("echo hello | nc -l -p %d", tcpPort)) //tcp

				Ω(otherContainer.StreamIn("bin/", tgzReader(netdogBin))).Should(Succeed())
				_, udpListenerOut = runInContainer(otherContainer, fmt.Sprintf("echo hello | ~/bin/netdog listen %d", udpPort)) //udp
				Eventually(udpListenerOut).Should(gbytes.Say("listening"))
			})

			Context("when the target address is inside DENY_NETWORKS", func() {
				BeforeEach(func() {
					denyRange = "10.0.0.0/8"
					allowRange = ""
					containerNetwork = fmt.Sprintf("10.1%d.0.0/24", GinkgoParallelNode())
				})

				Context("by default", func() {
					It("does not allow connections", func() {
						ByRejectingICMPPings()
						ByRejectingUDP()
						ByRejectingTCP()
					})
				})

				Context("after a net_out of another range", func() {
					It("still does not allow connections to that address", func() {
						Ω(container.NetOut(garden.NetOutRule{
							Networks: []garden.IPRange{
								IPRangeFromCIDR("1.2.3.4/30"),
							},
						})).Should(Succeed())
						ByRejectingICMPPings()
						ByRejectingUDP()
						ByRejectingTCP()
					})
				})

				Context("after net_out allows all traffic to that IP and port", func() {
					Context("when no port is specified", func() {
						It("allows both tcp and icmp to that address", func() {
							Ω(container.NetOut(garden.NetOutRule{
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
							})).Should(Succeed())
							ByAllowingICMPPings()
							ByAllowingUDP()
							ByAllowingTCP()
						})
					})

					Context("when a list of port ranges is specified", func() {
						It("allows tcp connections to the first port in that range", func() {
							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolTCP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRangeFromPort(12345),
									garden.PortRangeFromPort(9876),
								},
							})).Should(Succeed())
							ByRejectingTCP()

							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolTCP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRangeFromPort(tcpPort),
									garden.PortRangeFromPort(9876),
								},
							})).Should(Succeed())
							ByRejectingUDP()
							ByRejectingICMPPings()
							ByAllowingTCP()
						})

						It("allows tcp connections to the final port in that range", func() {
							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolTCP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRangeFromPort(9876),
									garden.PortRangeFromPort(12345),
								},
							})).Should(Succeed())
							ByRejectingTCP()

							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolTCP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRangeFromPort(9876),
									garden.PortRangeFromPort(tcpPort),
								},
							})).Should(Succeed())
							ByRejectingUDP()
							ByRejectingICMPPings()
							ByAllowingTCP()
						})
					})

					Context("when a port is specified", func() {
						It("allows tcp connections to that port", func() {
							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolTCP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRangeFromPort(12345),
								},
							})).Should(Succeed())
							ByRejectingTCP()

							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolTCP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRangeFromPort(tcpPort),
								},
							})).Should(Succeed())
							ByRejectingUDP()
							ByRejectingICMPPings()
							ByAllowingTCP()
						})

						It("allows udp connections to that port", func() {
							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolUDP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRangeFromPort(12345),
								},
							})).Should(Succeed())
							ByRejectingUDP()

							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolUDP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRangeFromPort(udpPort),
								},
							})).Should(Succeed())
							ByRejectingICMPPings()
							ByRejectingTCP()
							ByAllowingUDP()
						})
					})

					Context("when a port range is specified", func() {
						It("allows tcp connections a port in that range", func() {
							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolTCP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRange{tcpPort - 1, tcpPort + 1},
								},
							})).Should(Succeed())

							ByRejectingUDP()
							ByRejectingICMPPings()
							ByAllowingTCP()
						})

						It("allows udp connections a port in that range", func() {
							Ω(container.NetOut(garden.NetOutRule{
								Protocol: garden.ProtocolUDP,
								Networks: []garden.IPRange{
									garden.IPRangeFromIPNet(otherContainerNetwork),
								},
								Ports: []garden.PortRange{
									garden.PortRange{udpPort - 1, udpPort + 1},
								},
							})).Should(Succeed())

							ByRejectingTCP()
							ByRejectingICMPPings()
							ByAllowingUDP()
						})
					})
				})

				Describe("when no port or port is specified", func() {
					It("allows all TCP and rejects other protocols", func() {
						Ω(container.NetOut(garden.NetOutRule{
							Protocol: garden.ProtocolTCP,
							Networks: []garden.IPRange{
								garden.IPRangeFromIPNet(otherContainerNetwork),
							},
						})).Should(Succeed())
						ByAllowingTCP()
						ByRejectingICMPPings()
						ByRejectingUDP()
					})

					It("allows UDP and rejects other protocols", func() {
						Ω(container.NetOut(garden.NetOutRule{
							Protocol: garden.ProtocolUDP,
							Networks: []garden.IPRange{
								garden.IPRangeFromIPNet(otherContainerNetwork),
							},
						})).Should(Succeed())
						ByRejectingTCP()
						ByRejectingICMPPings()
						ByAllowingUDP()
					})
				})

				Describe("logging in TCP", func() {
					It("writes log entries to syslog when TCP requests are made", func() {
						tmpDir, err := ioutil.TempDir("", "iptables-log-test")
						Ω(err).ShouldNot(HaveOccurred())
						defer os.RemoveAll(tmpDir)

						configFile := filepath.Join(tmpDir, "ulogd.conf")
						logFile := filepath.Join(tmpDir, "ulogd.log")

						Ω(ioutil.WriteFile(configFile, []byte(`
[global]
logfile="syslog"
loglevel=1

plugin="/usr/lib/x86_64-linux-gnu/ulogd/ulogd_inppkt_NFLOG.so"
plugin="/usr/lib/x86_64-linux-gnu/ulogd/ulogd_output_LOGEMU.so"
plugin="/usr/lib/x86_64-linux-gnu/ulogd/ulogd_raw2packet_BASE.so"
plugin="/usr/lib/x86_64-linux-gnu/ulogd/ulogd_filter_IFINDEX.so"
plugin="/usr/lib/x86_64-linux-gnu/ulogd/ulogd_filter_IP2STR.so"
plugin="/usr/lib/x86_64-linux-gnu/ulogd/ulogd_filter_PRINTPKT.so"

stack=log1:NFLOG,base1:BASE,ifi1:IFINDEX,ip2str1:IP2STR,print1:PRINTPKT,emu1:LOGEMU

[log1]
group=1

[emu1]
file=`+logFile+`
sync=1
`), 0755)).Should(Succeed())

						pidFile := filepath.Join(tmpDir, "ulog.pid")
						ulogd, err := gexec.Start(exec.Command("ulogd", "-p", pidFile, "-c", configFile), GinkgoWriter, GinkgoWriter)
						Ω(err).ShouldNot(HaveOccurred())
						defer ulogd.Kill()

						Eventually(func() error { _, err := os.Stat(pidFile); return err }).ShouldNot(HaveOccurred(), "ulogd should write a pidfile")

						Ω(container.NetOut(garden.NetOutRule{
							Protocol: garden.ProtocolTCP,
							Networks: []garden.IPRange{
								garden.IPRangeFromIPNet(otherContainerNetwork),
							},
							Log: true,
						})).Should(Succeed())
						Ω(err).ShouldNot(HaveOccurred())

						// Use a large message to increase the probability that logging occurs.
						// Note that there is a 4096 byte buffer on ulog kernel module.
						ByAllowingTCP(strings.Repeat("a", 5000))

						logs := func() string {
							logs, err := ioutil.ReadFile(logFile)
							Ω(err).ShouldNot(HaveOccurred())
							return string(logs)
						}

						// the ulog kernel module can sometimes buffer messages for up to 15 seconds
						Eventually(logs, "15s").Should(MatchRegexp("%s.+DST=%s", regexp.QuoteMeta(container.Handle()), regexp.QuoteMeta(targetIP(otherContainer))))

						Consistently(func() []string {
							return regexp.MustCompile(fmt.Sprintf("%s.+DST=%s", regexp.QuoteMeta(container.Handle()), regexp.QuoteMeta(targetIP(otherContainer)))).FindAllString(logs(), -1)
						}, "1s").Should(HaveLen(1), "only the first packet should be logged")
					})
				})
			})

			Context("when the target address is inside ALLOW_NETWORKS", func() {
				BeforeEach(func() {
					containerNetwork = fmt.Sprintf("10.1%d.0.0/24", GinkgoParallelNode())
					denyRange = "10.0.0.0/8"
					allowRange = otherContainerNetwork.String()
				})

				It("allows connections", func() {
					ByAllowingTCP()
					ByAllowingUDP()
					ByAllowingICMPPings()
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
					ByAllowingUDP()
					ByAllowingICMPPings()
				})
			})
		})
	})
})

func tgzReader(path string) io.Reader {
	body, err := ioutil.ReadFile(path)
	Ω(err).ShouldNot(HaveOccurred())

	tmpdir, err := ioutil.TempDir("", "netdog")
	Ω(err).ShouldNot(HaveOccurred())

	tgzPath := filepath.Join(tmpdir, "netdog.tgz")

	archiver.CreateTarGZArchive(
		tgzPath,
		[]archiver.ArchiveFile{
			{
				Name: "./netdog",
				Body: string(body),
			},
		},
	)

	tgz, err := os.Open(tgzPath)
	Ω(err).ShouldNot(HaveOccurred())

	tarStream, err := gzip.NewReader(tgz)
	Ω(err).ShouldNot(HaveOccurred())

	return tarStream
}

func dumpIP() {
	cmd := exec.Command("ip", "a")
	op, err := cmd.CombinedOutput()
	Ω(err).ShouldNot(HaveOccurred())
	fmt.Println("IP status:\n", string(op))

	cmd = exec.Command("iptables", "--verbose", "--exact", "--numeric", "--list")
	op, err = cmd.CombinedOutput()
	Ω(err).ShouldNot(HaveOccurred())
	fmt.Println("IP tables chains:\n", string(op))

	cmd = exec.Command("iptables", "--list-rules")
	op, err = cmd.CombinedOutput()
	Ω(err).ShouldNot(HaveOccurred())
	fmt.Println("IP tables rules:\n", string(op))
}

func IPRangeFromCIDR(cidr string) garden.IPRange {
	_, ipn, err := net.ParseCIDR(cidr)
	Ω(err).ShouldNot(HaveOccurred())

	return garden.IPRangeFromIPNet(ipn)
}
