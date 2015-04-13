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
	var container *fakes.FakeContainerInitializer

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		hooks = make(hook.HookSet)
		config = process.Env{
			"id": "someID",
		}
		container = &fakes.FakeContainerInitializer{}
	})

	Context("After RegisterHooks has been run", func() {
		JustBeforeEach(func() {
			linux_backend.RegisterHooks(hooks, fakeRunner, config, container)
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
				It("runs the hook-parent-after-clone.sh legacy shell script", func() {
					hooks.Main(hook.PARENT_AFTER_CLONE)
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

		Context("Inside the child", func() {

			Context("after pivotting in to the rootfs", func() {
				It("sets the hostname to the container ID", func() {
					container.SetHostnameReturns(nil)
					hooks.Main(hook.CHILD_AFTER_PIVOT)
					Expect(container.SetHostnameCallCount()).To(Equal(1))
					Expect(container.SetHostnameArgsForCall(0)).To(Equal("someID"))
				})

				It("mounts proc", func() {
					container.MountProcReturns(nil)
					hooks.Main(hook.CHILD_AFTER_PIVOT)
					Expect(container.MountProcCallCount()).To(Equal(1))
				})

				It("mounts tmp", func() {
					container.MountTmpReturns(nil)
					hooks.Main(hook.CHILD_AFTER_PIVOT)
					Expect(container.MountTmpCallCount()).To(Equal(1))
				})
			})
		})
	})
})
