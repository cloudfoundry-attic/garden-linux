package iptables_test

import (
	"errors"
	"net"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/cloudfoundry-incubator/garden-linux/network/iptables"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Iptables", func() {
	Describe("Chain", func() {
		var fakeRunner *fake_command_runner.FakeCommandRunner
		var subject Chain
		var useKernelLogging bool

		JustBeforeEach(func() {
			fakeRunner = fake_command_runner.New()
			subject = NewLoggingChain("foo-bar-baz", useKernelLogging, fakeRunner, lagertest.NewTestLogger("test"))
		})

		Describe("Setup", func() {
			Context("when kernel logging is not enabled", func() {
				It("creates the log chain using iptables", func() {
					Ω(subject.Setup()).Should(Succeed())
					Ω(fakeRunner).Should(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-F", "foo-bar-baz-log"},
						},
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-X", "foo-bar-baz-log"},
						},
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-N", "foo-bar-baz-log"},
						},
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-A", "foo-bar-baz-log", "-m", "conntrack", "--ctstate", "NEW,UNTRACKED,INVALID", "--protocol", "tcp", "--jump", "NFLOG", "--nflog-prefix", "foo-bar-baz ", "--nflog-group", "1"},
						},
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-A", "foo-bar-baz-log", "--jump", "RETURN"},
						}))
				})
			})

			Context("when kernel logging is enabled", func() {
				BeforeEach(func() {
					useKernelLogging = true
				})

				It("creates the log chain using iptables", func() {
					Ω(subject.Setup()).Should(Succeed())
					Ω(fakeRunner).Should(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-F", "foo-bar-baz-log"},
						},
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-X", "foo-bar-baz-log"},
						},
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-N", "foo-bar-baz-log"},
						},
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-A", "foo-bar-baz-log", "-m", "conntrack", "--ctstate", "NEW,UNTRACKED,INVALID", "--protocol", "tcp",
								"--jump", "LOG", "--log-prefix", "foo-bar-baz "},
						},
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-A", "foo-bar-baz-log", "--jump", "RETURN"},
						}))
				})
			})

			It("ignores failures to flush", func() {
				someError := errors.New("y")
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-F", "foo-bar-baz-log"},
					},
					func(cmd *exec.Cmd) error {
						return someError
					})

				Ω(subject.Setup()).Should(Succeed())
			})

			It("ignores failures to delete", func() {
				someError := errors.New("y")
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-X", "foo-bar-baz-log"},
					},
					func(cmd *exec.Cmd) error {
						return someError
					})

				Ω(subject.Setup()).Should(Succeed())
			})

			It("returns any error returned when the table is created", func() {
				someError := errors.New("y")
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-N", "foo-bar-baz-log"},
					},
					func(cmd *exec.Cmd) error {
						return someError
					})

				Ω(subject.Setup()).Should(MatchError("iptables: log chain setup: y"))
			})

			It("returns any error returned when the logging rule is added", func() {
				someError := errors.New("y")
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-A", "foo-bar-baz-log", "-m", "conntrack", "--ctstate", "NEW,UNTRACKED,INVALID", "--protocol", "tcp", "--jump", "LOG", "--log-prefix", "foo-bar-baz "},
					},
					func(cmd *exec.Cmd) error {
						return someError
					})

				Ω(subject.Setup()).Should(MatchError("iptables: log chain setup: y"))
			})

			It("returns any error returned when the RETURN rule is added", func() {
				someError := errors.New("y")
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-A", "foo-bar-baz-log", "--jump", "RETURN"},
					},
					func(cmd *exec.Cmd) error {
						return someError
					})

				Ω(subject.Setup()).Should(MatchError("iptables: log chain setup: y"))
			})
		})

		Describe("TearDown", func() {
			It("should flush and delete the underlying iptables log chain", func() {
				Ω(subject.TearDown()).Should(Succeed())
				Ω(fakeRunner).Should(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-F", "foo-bar-baz-log"},
					},
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-X", "foo-bar-baz-log"},
					}))
			})

			It("ignores failures to flush", func() {
				someError := errors.New("y")
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-F", "foo-bar-baz-log"},
					},
					func(cmd *exec.Cmd) error {
						return someError
					})

				Ω(subject.TearDown()).Should(Succeed())
			})

			It("ignores failures to delete", func() {
				someError := errors.New("y")
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-X", "foo-bar-baz-log"},
					},
					func(cmd *exec.Cmd) error {
						return someError
					})

				Ω(subject.TearDown()).Should(Succeed())
			})

		})

		Describe("AppendRule", func() {
			It("runs iptables to create the rule with the correct parameters", func() {
				subject.AppendRule("", "2.0.0.0/11", Return)

				Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
					Path: "/sbin/iptables",
					Args: []string{"-w", "-A", "foo-bar-baz", "--destination", "2.0.0.0/11", "--jump", "RETURN"},
				}))
			})
		})

		Describe("AppendNatRule", func() {
			Context("creating a rule", func() {
				Context("when all parameters are specified", func() {
					It("runs iptables to create the rule with the correct parameters", func() {
						subject.AppendNatRule("1.3.5.0/28", "2.0.0.0/11", Return, net.ParseIP("1.2.3.4"))

						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-t", "nat", "-A", "foo-bar-baz", "--source", "1.3.5.0/28", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
						}))
					})
				})

				Context("when Source is not specified", func() {
					It("does not include the --source parameter in the command", func() {
						subject.AppendNatRule("", "2.0.0.0/11", Return, net.ParseIP("1.2.3.4"))

						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-t", "nat", "-A", "foo-bar-baz", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
						}))
					})
				})

				Context("when Destination is not specified", func() {
					It("does not include the --destination parameter in the command", func() {
						subject.AppendNatRule("1.3.5.0/28", "", Return, net.ParseIP("1.2.3.4"))

						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-t", "nat", "-A", "foo-bar-baz", "--source", "1.3.5.0/28", "--jump", "RETURN", "--to", "1.2.3.4"},
						}))
					})
				})

				Context("when To is not specified", func() {
					It("does not include the --to parameter in the command", func() {
						subject.AppendNatRule("1.3.5.0/28", "2.0.0.0/11", Return, nil)

						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-t", "nat", "-A", "foo-bar-baz", "--source", "1.3.5.0/28", "--destination", "2.0.0.0/11", "--jump", "RETURN"},
						}))
					})
				})

				Context("when the command returns an error", func() {
					It("returns an error", func() {
						someError := errors.New("badly laid iptable")
						fakeRunner.WhenRunning(
							fake_command_runner.CommandSpec{Path: "/sbin/iptables"},
							func(cmd *exec.Cmd) error {
								return someError
							},
						)

						Ω(subject.AppendRule("1.2.3.4/5", "", "")).ShouldNot(Succeed())
					})
				})
			})

			Describe("DeleteRule", func() {
				It("runs iptables to delete the rule with the correct parameters", func() {
					subject.DeleteRule("", "2.0.0.0/11", Return)

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-D", "foo-bar-baz", "--destination", "2.0.0.0/11", "--jump", "RETURN"},
					}))
				})
			})

			Context("DeleteNatRule", func() {
				Context("when all parameters are specified", func() {
					It("runs iptables to delete the rule with the correct parameters", func() {
						subject.DeleteNatRule("1.3.5.0/28", "2.0.0.0/11", Return, net.ParseIP("1.2.3.4"))

						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-t", "nat", "-D", "foo-bar-baz", "--source", "1.3.5.0/28", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
						}))
					})
				})

				Context("when Source is not specified", func() {
					It("does not include the --source parameter in the command", func() {
						subject.DeleteNatRule("", "2.0.0.0/11", Return, net.ParseIP("1.2.3.4"))

						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-t", "nat", "-D", "foo-bar-baz", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
						}))
					})
				})

				Context("when Destination is not specified", func() {
					It("does not include the --destination parameter in the command", func() {
						subject.DeleteNatRule("1.3.5.0/28", "", Return, net.ParseIP("1.2.3.4"))

						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-t", "nat", "-D", "foo-bar-baz", "--source", "1.3.5.0/28", "--jump", "RETURN", "--to", "1.2.3.4"},
						}))
					})
				})

				Context("when To is not specified", func() {
					It("does not include the --to parameter in the command", func() {
						subject.DeleteNatRule("1.3.5.0/28", "2.0.0.0/11", Return, nil)

						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-t", "nat", "-D", "foo-bar-baz", "--source", "1.3.5.0/28", "--destination", "2.0.0.0/11", "--jump", "RETURN"},
						}))
					})
				})

				Context("when the command returns an error", func() {
					It("returns an error", func() {
						someError := errors.New("badly laid iptable")
						fakeRunner.WhenRunning(
							fake_command_runner.CommandSpec{Path: "/sbin/iptables"},
							func(cmd *exec.Cmd) error {
								return someError
							},
						)

						Ω(subject.DeleteNatRule("1.3.4.5/6", "", "", nil)).ShouldNot(Succeed())
					})
				})
			})

			Describe("PrependFilterRule", func() {
				Context("when all parameters are defaulted", func() {
					It("runs iptables with appropriate parameters", func() {
						Ω(subject.PrependFilterRule(garden.NetOutRule{})).Should(Succeed())
						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--jump", "RETURN"},
						}))
					})
				})

				Describe("Network", func() {
					Context("when an empty IPRange is specified", func() {
						It("does not limit the range", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Networks: []garden.IPRange{
									garden.IPRange{},
								},
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--jump", "RETURN"},
							}))
						})
					})

					Context("when a single destination IP is specified", func() {
						It("opens only that IP", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Networks: []garden.IPRange{
									{
										Start: net.ParseIP("1.2.3.4"),
									},
								},
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination", "1.2.3.4", "--jump", "RETURN"},
							}))
						})
					})

					Context("when a multiple destination networks are specified", func() {
						It("opens only that IP", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Networks: []garden.IPRange{
									{
										Start: net.ParseIP("1.2.3.4"),
									},
									{
										Start: net.ParseIP("2.2.3.4"),
										End:   net.ParseIP("2.2.3.9"),
									},
								},
							})).Should(Succeed())

							Ω(fakeRunner.ExecutedCommands()).Should(HaveLen(2))
							Ω(fakeRunner).Should(HaveExecutedSerially(
								fake_command_runner.CommandSpec{
									Path: "/sbin/iptables",
									Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination", "1.2.3.4", "--jump", "RETURN"},
								},
								fake_command_runner.CommandSpec{
									Path: "/sbin/iptables",
									Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "-m", "iprange", "--dst-range", "2.2.3.4-2.2.3.9", "--jump", "RETURN"},
								},
							))
						})
					})

					Context("when a EndIP is specified without a StartIP", func() {
						It("opens only that IP", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Networks: []garden.IPRange{
									{
										End: net.ParseIP("1.2.3.4"),
									},
								},
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination", "1.2.3.4", "--jump", "RETURN"},
							}))
						})
					})

					Context("when a range of IPs is specified", func() {
						It("opens only the range", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Networks: []garden.IPRange{
									{
										net.ParseIP("1.2.3.4"), net.ParseIP("2.3.4.5"),
									},
								},
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "-m", "iprange", "--dst-range", "1.2.3.4-2.3.4.5", "--jump", "RETURN"},
							}))
						})
					})
				})

				Describe("Ports", func() {
					Context("when a single port is specified", func() {
						It("opens only that port", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Ports: []garden.PortRange{
									garden.PortRangeFromPort(22),
								},
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination-port", "22", "--jump", "RETURN"},
							}))
						})
					})

					Context("when a port range is specified", func() {
						It("opens that port range", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Ports: []garden.PortRange{
									{12, 24},
								},
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination-port", "12:24", "--jump", "RETURN"},
							}))
						})
					})

					Context("when multiple port ranges are specified", func() {
						It("opens those port ranges", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Ports: []garden.PortRange{
									{12, 24},
									{64, 942},
								},
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(
								fake_command_runner.CommandSpec{
									Path: "/sbin/iptables",
									Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination-port", "12:24", "--jump", "RETURN"},
								},
								fake_command_runner.CommandSpec{
									Path: "/sbin/iptables",
									Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination-port", "64:942", "--jump", "RETURN"},
								},
							))
						})
					})
				})

				Describe("Protocol", func() {
					Context("when tcp protocol is specified", func() {
						It("passes tcp protocol to iptables", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Protocol: garden.ProtocolTCP,
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "tcp", "--jump", "RETURN"},
							}))
						})
					})

					Context("when udp protocol is specified", func() {
						It("passes udp protocol to iptables", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Protocol: garden.ProtocolUDP,
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "udp", "--jump", "RETURN"},
							}))
						})
					})

					Context("when icmp protocol is specified", func() {
						It("passes icmp protocol to iptables", func() {
							Ω(subject.PrependFilterRule(garden.NetOutRule{
								Protocol: garden.ProtocolICMP,
							})).Should(Succeed())

							Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "icmp", "--jump", "RETURN"},
							}))
						})

						Context("when icmp type is specified", func() {
							It("passes icmp protcol type to iptables", func() {
								Ω(subject.PrependFilterRule(garden.NetOutRule{
									Protocol: garden.ProtocolICMP,
									ICMPs: &garden.ICMPControl{
										Type: 99,
									},
								})).Should(Succeed())

								Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
									Path: "/sbin/iptables",
									Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "icmp", "--icmp-type", "99", "--jump", "RETURN"},
								}))
							})
						})

						Context("when icmp type and code are specified", func() {
							It("passes icmp protcol type and code to iptables", func() {
								Ω(subject.PrependFilterRule(garden.NetOutRule{
									Protocol: garden.ProtocolICMP,
									ICMPs: &garden.ICMPControl{
										Type: 99,
										Code: garden.ICMPControlCode(11),
									},
								})).Should(Succeed())

								Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
									Path: "/sbin/iptables",
									Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "icmp", "--icmp-type", "99/11", "--jump", "RETURN"},
								}))
							})
						})
					})
				})

				Describe("Log", func() {
					It("redirects via the log chain if log is specified", func() {
						Ω(subject.PrependFilterRule(garden.NetOutRule{
							Log: true,
						})).Should(Succeed())

						Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
							Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--goto", "foo-bar-baz-log"},
						}))
					})
				})

				Context("when multiple port ranges and multiple networks are specified", func() {
					It("opens the permutations of those port ranges and networks", func() {
						Ω(subject.PrependFilterRule(garden.NetOutRule{
							Networks: []garden.IPRange{
								{
									Start: net.ParseIP("1.2.3.4"),
								},
								{
									Start: net.ParseIP("2.2.3.4"),
									End:   net.ParseIP("2.2.3.9"),
								},
							},
							Ports: []garden.PortRange{
								{12, 24},
								{64, 942},
							},
						})).Should(Succeed())

						Ω(fakeRunner.ExecutedCommands()).Should(HaveLen(4))
						Ω(fakeRunner).Should(HaveExecutedSerially(
							fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination", "1.2.3.4", "--destination-port", "12:24", "--jump", "RETURN"},
							},
							fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination", "1.2.3.4", "--destination-port", "64:942", "--jump", "RETURN"},
							},
							fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "-m", "iprange", "--dst-range", "2.2.3.4-2.2.3.9", "--destination-port", "12:24", "--jump", "RETURN"},
							},
							fake_command_runner.CommandSpec{
								Path: "/sbin/iptables",
								Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "-m", "iprange", "--dst-range", "2.2.3.4-2.2.3.9", "--destination-port", "64:942", "--jump", "RETURN"},
							},
						))
					})
				})

				Context("when an invaild protocol is specified", func() {
					It("returns an error", func() {
						err := subject.PrependFilterRule(garden.NetOutRule{
							Protocol: garden.Protocol(52),
						})
						Ω(err).Should(HaveOccurred())
						Ω(err).Should(MatchError("invalid protocol: 52"))
					})
				})

				Context("when the command returns an error", func() {
					It("returns a wrapped error, including stderr", func() {
						someError := errors.New("badly laid iptable")
						fakeRunner.WhenRunning(
							fake_command_runner.CommandSpec{Path: "/sbin/iptables"},
							func(cmd *exec.Cmd) error {
								cmd.Stderr.Write([]byte("stderr contents"))
								return someError
							},
						)

						Ω(subject.PrependFilterRule(garden.NetOutRule{})).Should(MatchError("iptables: badly laid iptable, stderr contents"))
					})
				})
			})
		})
	})
})
