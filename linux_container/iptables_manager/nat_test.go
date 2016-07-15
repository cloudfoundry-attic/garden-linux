package iptables_manager_test

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"

	"net"

	"code.cloudfoundry.org/garden-linux/linux_container/iptables_manager"
	"code.cloudfoundry.org/garden-linux/sysconfig"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"code.cloudfoundry.org/lager/lagertest"
)

var _ = Describe("natChain", func() {
	var (
		fakeRunner  *fake_command_runner.FakeCommandRunner
		testCfg     *sysconfig.IPTablesNATConfig
		chain       iptables_manager.Chain
		containerID string
		bridgeName  string
		ip          net.IP
		network     *net.IPNet
	)

	BeforeEach(func() {
		var err error

		fakeRunner = fake_command_runner.New()
		testCfg = &sysconfig.IPTablesNATConfig{
			PreroutingChain:  "nat-prerouting-chain",
			PostroutingChain: "nat-postrouting-chain",
			InstancePrefix:   "nat-instance-prefix",
		}

		containerID = "some-ctr-id"
		bridgeName = "some-bridge"
		ip, network, err = net.ParseCIDR("1.2.3.4/28")
		Expect(err).NotTo(HaveOccurred())

		chain = iptables_manager.NewNATChain(testCfg, fakeRunner, lagertest.NewTestLogger("test"))
	})

	Describe("ContainerSetup", func() {
		var specs []fake_command_runner.CommandSpec
		BeforeEach(func() {
			expectedNatInstanceChain := testCfg.InstancePrefix + containerID
			specs = []fake_command_runner.CommandSpec{
				fake_command_runner.CommandSpec{
					Path: "iptables",
					Args: []string{"--wait", "--table", "nat", "-N", expectedNatInstanceChain},
				},
				fake_command_runner.CommandSpec{
					Path: "iptables",
					Args: []string{"--wait", "--table", "nat", "-A", testCfg.PreroutingChain,
						"--jump", expectedNatInstanceChain},
				},
				fake_command_runner.CommandSpec{
					Path: "sh",
					Args: []string{"-c", fmt.Sprintf(
						`(iptables --wait --table nat -S %s | grep "\-j MASQUERADE\b" | grep -q -F -- "-s %s") || iptables --wait --table nat -A %s --source %s ! --destination %s --jump MASQUERADE`,
						testCfg.PostroutingChain, network.String(), testCfg.PostroutingChain,
						network.String(), network.String(),
					)},
				},
			}
		})

		It("should set up the chain", func() {
			Expect(chain.Setup(containerID, bridgeName, ip, network)).To(Succeed())

			Expect(fakeRunner).To(HaveExecutedSerially(specs...))
		})

		DescribeTable("iptables failures",
			func(specIndex int, errorString string) {
				fakeRunner.WhenRunning(specs[specIndex], func(*exec.Cmd) error {
					return errors.New("iptables failed")
				})

				Expect(chain.Setup(containerID, bridgeName, ip, network)).To(MatchError(errorString))
			},
			Entry("create nat instance chain", 0, "iptables_manager: nat: iptables failed"),
			Entry("bind nat instance chain to nat prerouting chain", 1, "iptables_manager: nat: iptables failed"),
			Entry("enable NAT for traffic coming from containers", 2, "iptables_manager: nat: iptables failed"),
		)
	})

	Describe("ContainerTeardown", func() {
		var specs []fake_command_runner.CommandSpec

		Describe("nat chain", func() {
			BeforeEach(func() {
				expectedFilterInstanceChain := testCfg.InstancePrefix + containerID
				specs = []fake_command_runner.CommandSpec{
					fake_command_runner.CommandSpec{
						Path: "sh",
						Args: []string{"-c", fmt.Sprintf(
							`iptables --wait --table nat -S %s 2> /dev/null | grep "\-j %s\b" | sed -e "s/-A/-D/" | xargs --no-run-if-empty --max-lines=1 iptables --wait --table nat`,
							testCfg.PreroutingChain, expectedFilterInstanceChain,
						)},
					},
					fake_command_runner.CommandSpec{
						Path: "sh",
						Args: []string{"-c", fmt.Sprintf(
							`iptables --wait --table nat -F %s 2> /dev/null || true`,
							expectedFilterInstanceChain,
						)},
					},
					fake_command_runner.CommandSpec{
						Path: "sh",
						Args: []string{"-c", fmt.Sprintf(
							`iptables --wait --table nat -X %s 2> /dev/null || true`,
							expectedFilterInstanceChain,
						)},
					},
				}
			})

			It("should tear down the chain", func() {
				Expect(chain.Teardown(containerID)).To(Succeed())

				Expect(fakeRunner).To(HaveExecutedSerially(specs...))
			})

			DescribeTable("iptables failures",
				func(specIndex int, errorString string) {
					fakeRunner.WhenRunning(specs[specIndex], func(*exec.Cmd) error {
						return errors.New("iptables failed")
					})

					Expect(chain.Teardown(containerID)).To(MatchError(errorString))
				},
				Entry("prune prerouting chain", 0, "iptables_manager: nat: iptables failed"),
				Entry("flush instance chain", 1, "iptables_manager: nat: iptables failed"),
				Entry("delete instance chain", 2, "iptables_manager: nat: iptables failed"),
			)
		})
	})
})
