package linux_backend_test

import (
	"errors"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/hook"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"

	"os"

	"net"

	networkFakes "github.com/cloudfoundry-incubator/garden-linux/network/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hooks", func() {
	var hooks hook.HookSet
	var fakeRunner *fake_command_runner.FakeCommandRunner
	var config process.Env
	var fakeNetworkConfigurer *networkFakes.FakeConfigurer

	BeforeEach(func() {
		hooks = make(hook.HookSet)
		fakeRunner = fake_command_runner.New()
		config = process.Env{
			"id":                      "someID",
			"network_cidr":            "1.2.3.4/8",
			"container_iface_mtu":     "5000",
			"network_container_ip":    "1.6.6.6",
			"network_host_ip":         "1.2.3.5",
			"network_host_iface":      "hostIfc",
			"network_container_iface": "containerIfc",
			"bridge_iface":            "bridgeName",
		}
		fakeNetworkConfigurer = &networkFakes.FakeConfigurer{}
	})

	Context("After RegisterHooks has been run", func() {
		JustBeforeEach(func() {
			linux_backend.RegisterHooks(hooks, fakeRunner, config, fakeNetworkConfigurer)
		})

		Context("Inside the host", func() {
			Context("before container creation", func() {
				It("runs the hook-parent-before-clone.sh legacy shell script", func() {
					hooks.Main(hook.PARENT_BEFORE_CLONE)
					Expect(fakeRunner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "hook-parent-before-clone.sh",
					}))
				})

				Context("when the legacy shell script fails", func() {
					BeforeEach(func() {
						fakeRunner.WhenRunning(fake_command_runner.CommandSpec{
							Path: "hook-parent-before-clone.sh",
						}, func(*exec.Cmd) error {
							return errors.New("o no")
						})
					})

					It("panics", func() {
						Expect(func() { hooks.Main(hook.PARENT_BEFORE_CLONE) }).To(Panic())
					})
				})
			})

			Context("after container creation", func() {
				var oldWd, testDir string

				BeforeEach(func() {
					os.Setenv("PID", "99")
				})

				AfterEach(func() {
					if oldWd != "" {
						os.Chdir(oldWd)
					}

					if testDir != "" {
						os.RemoveAll(testDir)
					}
				})

				It("configures the host's network correctly", func() {
					Expect(func() { hooks.Main(hook.PARENT_AFTER_CLONE) }).ToNot(Panic())

					Expect(fakeNetworkConfigurer.ConfigureHostCallCount()).To(Equal(1))
					hostConfig := fakeNetworkConfigurer.ConfigureHostArgsForCall(0)
					Expect(hostConfig.HostIntf).To(Equal("hostIfc"))
					Expect(hostConfig.ContainerIntf).To(Equal("containerIfc"))
					Expect(hostConfig.BridgeName).To(Equal("bridgeName"))
					Expect(hostConfig.ContainerPid).To(Equal(99))
					Expect(hostConfig.BridgeIP).To(Equal(net.ParseIP("1.2.3.5")))
					_, expectedSubnet, _ := net.ParseCIDR("1.2.3.4/8")
					Expect(hostConfig.Subnet).To(Equal(expectedSubnet))
					Expect(hostConfig.Mtu).To(Equal(5000))
				})

				Context("when the network configurer fails", func() {
					BeforeEach(func() {
						fakeNetworkConfigurer.ConfigureHostReturns(errors.New("oh no!"))
					})

					It("panics", func() {
						Expect(func() { hooks.Main(hook.PARENT_AFTER_CLONE) }).To(Panic())
					})
				})

				Context("when the network CIDR is badly formatted", func() {
					BeforeEach(func() {
						config["network_cidr"] = "1.2.3.4/8/9"
					})

					It("panics", func() {
						Expect(func() { hooks.Main(hook.PARENT_AFTER_CLONE) }).To(Panic())
					})
				})

				Context("when the MTU is invalid", func() {
					BeforeEach(func() {
						config["container_iface_mtu"] = "x"
					})

					It("panics", func() {
						Expect(func() { hooks.Main(hook.PARENT_AFTER_CLONE) }).To(Panic())
					})
				})

				It("runs the hook-parent-after-clone.sh legacy shell script", func() {
					Expect(func() { hooks.Main(hook.PARENT_AFTER_CLONE) }).ToNot(Panic())
					Expect(fakeRunner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "hook-parent-after-clone.sh",
					}))
				})

				Context("when the legacy shell script fails", func() {
					BeforeEach(func() {
						fakeRunner.WhenRunning(fake_command_runner.CommandSpec{
							Path: "hook-parent-after-clone.sh",
						}, func(*exec.Cmd) error {
							return errors.New("o no")
						})
					})

					It("panics", func() {
						Expect(func() { hooks.Main(hook.PARENT_AFTER_CLONE) }).To(Panic())
					})
				})
			})
		})
	})
})
