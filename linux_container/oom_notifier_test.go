package linux_container_test

import (
	"errors"
	"os/exec"
	"path"
	"runtime/pprof"

	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("OomNotifier", func() {
	var (
		runner         *fake_command_runner.FakeCommandRunner
		cgroupsPath    string
		cgroupsManager linux_container.CgroupsManager
		oNoom          func()
		oomChan        chan struct{}
		containerPath  string
		oomNotifier    *linux_container.OomNotifier
	)

	BeforeEach(func() {
		runner = fake_command_runner.New()

		cgroupsPath = path.Join("path", "to", "cgroups")
		cgroupsManager = fake_cgroups_manager.New(cgroupsPath, "123456")

		containerPath = path.Join("path", "to", "container")

		oomChan = make(chan struct{})
		oNoom = func() {
			close(oomChan)
		}
	})

	JustBeforeEach(func() {
		oomNotifier = linux_container.NewOomNotifier(
			runner,
			containerPath,
			cgroupsManager,
		)
	})

	Describe("Watch", func() {
		It("calls the oom binary", func() {
			Expect(oomNotifier.Watch(oNoom)).To(Succeed())

			Expect(runner).To(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: path.Join(containerPath, "bin", "oom"),
					Args: []string{path.Join(cgroupsPath, "memory", "instance-123456")},
				},
			))

			Eventually(oomChan).Should(BeClosed())
		})

		Context("when the notifier is set", func() {
			Context("when the oom process exits with exit code 0", func() {
				It("notifies", func() {
					Expect(oomNotifier.Watch(oNoom)).To(Succeed())

					Eventually(oomChan).Should(BeClosed())
				})
			})

			Context("when the oom process does not exit", func() {
				var waitReturns chan struct{}

				BeforeEach(func() {
					waitReturns = make(chan struct{})

					runner.WhenWaitingFor(
						fake_command_runner.CommandSpec{
							Path: path.Join(containerPath, "bin", "oom"),
						},
						func(cmd *exec.Cmd) error {
							<-waitReturns
							return nil
						},
					)
				})

				AfterEach(func() {
					close(waitReturns)

					Eventually(oomChan).Should(BeClosed())
				})

				It("does not notify", func() {
					Expect(oomNotifier.Watch(oNoom)).To(Succeed())

					Consistently(oomChan).ShouldNot(BeClosed())
				})
			})

			Context("when the oom process exits with a non-zero exit code", func() {
				BeforeEach(func() {
					runner.WhenWaitingFor(
						fake_command_runner.CommandSpec{
							Path: path.Join(containerPath, "bin", "oom"),
						},
						func(cmd *exec.Cmd) error {
							return errors.New("banana")
						},
					)
				})

				It("does not call back", func() {
					Expect(oomNotifier.Watch(oNoom)).To(Succeed())

					Consistently(oomChan).ShouldNot(BeClosed())
				})

				It("should not leak goroutines", func() {
					Expect(oomNotifier.Watch(oNoom)).To(Succeed())

					Eventually(func() []byte {
						buffer := gbytes.NewBuffer()
						defer buffer.Close()
						Expect(pprof.Lookup("goroutine").WriteTo(buffer, 1)).To(Succeed())

						return buffer.Contents()
					}).ShouldNot(ContainSubstring("oom_notifier.go"))
				})
			})
		})
	})

	Describe("Unwatch", func() {
		Context("when oom has already occurred", func() {
			JustBeforeEach(func() {
				oomNotifier.Watch(oNoom)
			})

			It("should not kill the oom command", func() {
				Eventually(oomChan).Should(BeClosed())

				oomNotifier.Unwatch()

				Expect(runner.KilledCommands()).To(HaveLen(0))
			})
		})

		Context("when oom has not already occurred", func() {
			BeforeEach(func() {
				runner.WhenWaitingFor(
					fake_command_runner.CommandSpec{},
					func(cmd *exec.Cmd) error {
						return errors.New("Command got killed")
					})
			})

			It("kills the oom process", func() {
				Expect(oomNotifier.Watch(oNoom)).To(Succeed())

				oomNotifier.Unwatch()

				startedCommands := runner.StartedCommands()
				killedCommands := runner.KilledCommands()

				Expect(startedCommands).To(HaveLen(1))
				Expect(startedCommands).To(Equal(killedCommands))
			})
		})
	})
})
