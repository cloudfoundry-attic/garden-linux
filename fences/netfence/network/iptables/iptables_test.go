package iptables_test

import (
	"errors"
	"net"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/iptables"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Iptables", func() {
	var fakeRunner *fake_command_runner.FakeCommandRunner
	var subject Chain

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		subject = NewChainFactory(fakeRunner, lagertest.NewTestLogger("test")).CreateChain("foo-bar-baz")
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
			Context("when all parameters are specified", func() {
				It("runs iptables to prepend the rule with the correct parameters when port is specified", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolAll, "1.2.3.4/24", 8080, "", -1, -1)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination", "1.2.3.4/24", "--destination-port", "8080", "--jump", "RETURN"},
					}))
				})

				It("runs iptables to prepend the rule with the correct parameters when port range is specified", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolAll, "1.2.3.4/24", 0, "80:81", -1, -1)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination", "1.2.3.4/24", "--destination-port", "80:81", "--jump", "RETURN"},
					}))
				})
			})

			Context("when tcp protcol is specified", func() {
				It("passes tcp protcol to iptables", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolTCP, "1.2.3.4/24", 8080, "", -1, -1)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "tcp", "--destination", "1.2.3.4/24", "--destination-port", "8080", "--jump", "RETURN"},
					}))
				})
			})

			Context("when udp protcol is specified", func() {
				It("passes udp protcol to iptables", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolUDP, "1.2.3.4/24", 8080, "", -1, -1)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "udp", "--destination", "1.2.3.4/24", "--destination-port", "8080", "--jump", "RETURN"},
					}))
				})
			})

			Context("when icmp protcol is specified", func() {
				It("passes icmp protcol to iptables when no type or code is specified", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolICMP, "1.2.3.4/24", 0, "", -1, -1)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "icmp", "--destination", "1.2.3.4/24", "--jump", "RETURN"},
					}))
				})

				It("passes icmp protcol to iptables with icmp type if specified", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolICMP, "1.2.3.4/24", 0, "", 8, -1)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "icmp", "--destination", "1.2.3.4/24", "--icmp-type", "8", "--jump", "RETURN"},
					}))
				})

				It("passes icmp protcol to iptables with icmp type and code if both are specified", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolICMP, "1.2.3.4/24", 0, "", 8, 7)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "icmp", "--destination", "1.2.3.4/24", "--icmp-type", "8/7", "--jump", "RETURN"},
					}))
				})
			})

			Context("when destination is omitted", func() {
				It("does not pass destination to iptables", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolAll, "", 8080, "", -1, -1)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination-port", "8080", "--jump", "RETURN"},
					}))
				})
			})

			Context("when port is omitted", func() {
				It("does not pass port to iptables", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolAll, "1.2.3.4/24", 0, "", -1, -1)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "--destination", "1.2.3.4/24", "--jump", "RETURN"},
					}))
				})
			})

			Context("when an IP range is specified", func() {
				It("runs iptables to prepend the rule with the correct parameters", func() {
					Ω(subject.PrependFilterRule(garden.ProtocolAll, "1.2.3.4-1.2.3.6", 8080, "", -1, -1)).Should(Succeed())

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-I", "foo-bar-baz", "1", "--protocol", "all", "-m", "iprange", "--dst-range", "1.2.3.4-1.2.3.6", "--destination-port", "8080", "--jump", "RETURN"},
					}))
				})
			})

			Context("when an invaild protocol is specified", func() {
				It("returns an error", func() {
					err := subject.PrependFilterRule(garden.Protocol(52), "1.2.3.4/24", 8080, "", -1, -1)
					Ω(err).Should(HaveOccurred())
					Ω(err).Should(MatchError("invalid protocol: 52"))
				})
			})

			Context("when port and port range are specified", func() {
				It("returns an error", func() {
					err := subject.PrependFilterRule(garden.ProtocolTCP, "1.2.3.4/24", 8080, "80:81", -1, -1)
					Ω(err).Should(HaveOccurred())
					Ω(err).Should(MatchError("port 8080 and port range 80:81 cannot both be specified"))
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

					Ω(subject.PrependFilterRule(garden.ProtocolAll, "1.3.4.5/6", 0, "", -1, -1)).ShouldNot(Succeed())
				})
			})
		})
	})
})
