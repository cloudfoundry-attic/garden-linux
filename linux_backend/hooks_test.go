package linux_backend_test

import (
	"errors"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/hook"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"

	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hooks", func() {
	var hooks hook.HookSet
	var fakeRunner *fake_command_runner.FakeCommandRunner

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		hooks = make(hook.HookSet)
	})

	Context("After RegisterHooks has been run", func() {
		JustBeforeEach(func() {
			linux_backend.RegisterHooks(hooks, fakeRunner)
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

		Context("Inside the child", func() {
			Context("before pivotting in to the rootfs", func() {
				It("runs the hook-child-before-pivot.sh legacy shell script", func() {
					hooks.Main(hook.CHILD_BEFORE_PIVOT)
					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "hook-child-before-pivot.sh",
					}))
				})

				Context("when the legacy shell script fails", func() {
					BeforeEach(func() {
						fakeRunner.WhenRunning(fake_command_runner.CommandSpec{
							Path: "hook-child-before-pivot.sh",
						}, func(*exec.Cmd) error {
							return errors.New("o no")
						})
					})

					It("panics", func() {
						Ω(func() { hooks.Main(hook.CHILD_BEFORE_PIVOT) }).Should(Panic())
					})
				})
			})

			Context("after pivotting in to the rootfs", func() {
				It("runs the hook-child-after-pivot.sh legacy shell script", func() {
					hooks.Main(hook.CHILD_AFTER_PIVOT)
					Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
						Path: "hook-child-after-pivot.sh",
					}))
				})

				Context("when the legacy shell script fails", func() {
					BeforeEach(func() {
						fakeRunner.WhenRunning(fake_command_runner.CommandSpec{
							Path: "hook-child-after-pivot.sh",
						}, func(*exec.Cmd) error {
							return errors.New("o no")
						})
					})

					It("panics", func() {
						Ω(func() { hooks.Main(hook.CHILD_AFTER_PIVOT) }).Should(Panic())
					})
				})
			})
		})
	})
})
