package iptables_test

import (
	"errors"
	"net"
	"os/exec"

	. "github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/iptables"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Iptables", func() {
	var fakeRunner *fake_command_runner.FakeCommandRunner
	var subject *Chain

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		subject = &Chain{"foo-bar-baz", fakeRunner}
	})

	Describe("NATRule", func() {
		Context("creating a rule", func() {
			Context("when all parameters are specified", func() {
				It("runs iptables to create the rule with the correct parameters", func() {
					subject.Create(&Rule{
						Type:        Nat,
						Source:      "1.3.5.0/28",
						Destination: "2.0.0.0/11",
						Jump:        Return,
						To:          net.ParseIP("1.2.3.4"),
					})

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-t", "nat", "-A", "foo-bar-baz", "--source", "1.3.5.0/28", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
					}))
				})
			})

			Context("when Type is not specified", func() {
				It("does not include the -t parameter in the command", func() {
					subject.Create(&Rule{
						Jump:        Return,
						Destination: "2.0.0.0/11",
						To:          net.ParseIP("1.2.3.4"),
					})

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-A", "foo-bar-baz", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
					}))
				})
			})

			Context("when Source is not specified", func() {
				It("does not include the --source parameter in the command", func() {
					subject.Create(&Rule{
						Type:        Nat,
						Jump:        Return,
						Destination: "2.0.0.0/11",
						To:          net.ParseIP("1.2.3.4"),
					})

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-t", "nat", "-A", "foo-bar-baz", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
					}))
				})
			})

			Context("when Destination is not specified", func() {
				It("does not include the --destination parameter in the command", func() {
					subject.Create(&Rule{
						Type:   Nat,
						Source: "1.3.5.0/28",
						Jump:   Return,
						To:     net.ParseIP("1.2.3.4"),
					})

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-t", "nat", "-A", "foo-bar-baz", "--source", "1.3.5.0/28", "--jump", "RETURN", "--to", "1.2.3.4"},
					}))
				})
			})

			Context("when To is not specified", func() {
				It("does not include the --to parameter in the command", func() {
					subject.Create(&Rule{
						Type:        Nat,
						Source:      "1.3.5.0/28",
						Destination: "2.0.0.0/11",
						Jump:        Return,
					})

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

					Ω(subject.Create(&Rule{Source: "1.2.3.4/5"})).ShouldNot(Succeed())
				})
			})
		})

		Context("deleting a rule", func() {
			Context("when all parameters are specified", func() {
				It("runs iptables to delete the rule with the correct parameters", func() {
					subject.Destroy(&Rule{
						Type:        Nat,
						Source:      "1.3.5.0/28",
						Destination: "2.0.0.0/11",
						Jump:        Return,
						To:          net.ParseIP("1.2.3.4"),
					})

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-t", "nat", "-D", "foo-bar-baz", "--source", "1.3.5.0/28", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
					}))
				})
			})

			Context("when Type is not specified", func() {
				It("does not include the -t parameter in the command", func() {
					subject.Destroy(&Rule{
						Jump:        Return,
						Destination: "2.0.0.0/11",
						To:          net.ParseIP("1.2.3.4"),
					})

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-D", "foo-bar-baz", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
					}))
				})
			})

			Context("when Source is not specified", func() {
				It("does not include the --source parameter in the command", func() {
					subject.Destroy(&Rule{
						Type:        Nat,
						Jump:        Return,
						Destination: "2.0.0.0/11",
						To:          net.ParseIP("1.2.3.4"),
					})

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-t", "nat", "-D", "foo-bar-baz", "--destination", "2.0.0.0/11", "--jump", "RETURN", "--to", "1.2.3.4"},
					}))
				})
			})

			Context("when Destination is not specified", func() {
				It("does not include the --destination parameter in the command", func() {
					subject.Destroy(&Rule{
						Type:   Nat,
						Source: "1.3.5.0/28",
						Jump:   Return,
						To:     net.ParseIP("1.2.3.4"),
					})

					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-t", "nat", "-D", "foo-bar-baz", "--source", "1.3.5.0/28", "--jump", "RETURN", "--to", "1.2.3.4"},
					}))
				})
			})

			Context("when To is not specified", func() {
				It("does not include the --to parameter in the command", func() {
					subject.Destroy(&Rule{
						Type:        Nat,
						Source:      "1.3.5.0/28",
						Destination: "2.0.0.0/11",
						Jump:        Return,
					})

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

					Ω(subject.Destroy(&Rule{Source: "1.3.4.5/6"})).ShouldNot(Succeed())
				})
			})
		})
	})
})
