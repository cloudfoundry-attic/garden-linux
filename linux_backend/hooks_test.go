package linux_backend_test

import (
	"errors"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/hook"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/fakes"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"

	"github.com/cloudfoundry-incubator/garden-linux/process"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hooks", func() {
	var hooks hook.HookSet
	var fakeRunner *fake_command_runner.FakeCommandRunner
	var config process.Env
	var container *fakes.FakeRunningContainer

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		hooks = make(hook.HookSet)
		config = process.Env{
			"id": "someID",
		}
		container = &fakes.FakeRunningContainer{}
	})

	Context("After RegisterHooks has been run", func() {
		JustBeforeEach(func() {
			linux_backend.RegisterHooks(hooks, fakeRunner, config, container)
		})

		Context("Inside the host", func() {
			Context("before container creation", func() {
				It("runs the hook-parent-before-clone.sh legacy shell script", func() {
					hooks.Main(hook.PARENT_BEFORE_CLONE)
					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
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
						Ω(func() { hooks.Main(hook.PARENT_BEFORE_CLONE) }).Should(Panic())
					})
				})
			})

			Context("after container creation", func() {
				It("runs the hook-parent-after-clone.sh legacy shell script", func() {
					hooks.Main(hook.PARENT_AFTER_CLONE)
					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
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
						Ω(func() { hooks.Main(hook.PARENT_AFTER_CLONE) }).Should(Panic())
					})
				})
			})
		})

		// SPIKE to demo garden on CentOS variant: broke hook-child-after-pivot
		// should not be merged with broken tests
		XContext("Inside the child", func() {

			Context("after pivotting in to the rootfs", func() {
				It("sets the hostname to the container ID", func() {
					container.SetHostnameReturns(nil)
					hooks.Main(hook.CHILD_AFTER_PIVOT)
					Ω(container.SetHostnameCallCount()).Should(Equal(1))
					Ω(container.SetHostnameArgsForCall(0)).Should(Equal("someID"))
				})

				It("mounts proc", func() {
					container.MountProcReturns(nil)
					hooks.Main(hook.CHILD_AFTER_PIVOT)
					Ω(container.MountProcCallCount()).Should(Equal(1))
				})

				It("mounts tmp", func() {
					container.MountTmpReturns(nil)
					hooks.Main(hook.CHILD_AFTER_PIVOT)
					Ω(container.MountTmpCallCount()).Should(Equal(1))
				})
			})
		})
	})
})
