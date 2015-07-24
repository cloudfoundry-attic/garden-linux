package linux_container_test

import (
	"errors"
	"os/exec"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("OomNotifier", func() {
	var (
		runner         *fake_command_runner.FakeCommandRunner
		cgroupsPath    string
		cgroupsManager linux_container.CgroupsManager
		oom            chan struct{}
		containerPath  string
		oomNotifier    *linux_container.OomNotifier
	)

	BeforeEach(func() {
		runner = fake_command_runner.New()

		cgroupsPath = path.Join("path", "to", "cgroups")
		cgroupsManager = fake_cgroups_manager.New(cgroupsPath, "123456")

		containerPath = path.Join("path", "to", "container")

		oom = make(chan struct{})
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
			Expect(oomNotifier.Watch(oom)).To(Succeed())

			Expect(runner).To(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: path.Join(containerPath, "bin", "oom"),
					Args: []string{path.Join(cgroupsPath, "memory", "instance-123456")},
				},
			))
		})

		Context("when the notifier is set", func() {
			Context("when the oom process exits with exit code 0", func() {
				It("notifies", func() {
					Expect(oomNotifier.Watch(oom)).To(Succeed())

					Eventually(oom).Should(BeClosed())
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
				})

				It("does not notify", func() {
					Expect(oomNotifier.Watch(oom)).To(Succeed())

					Consistently(oom).ShouldNot(BeClosed())
				})
			})

			Context("when the oom process exists with exit code 1", func() {
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

				It("does not notify", func() {
					Expect(oomNotifier.Watch(oom)).To(Succeed())

					Consistently(oom).ShouldNot(BeClosed())
				})
			})
		})
	})

	Describe("Unwatch", func() {
		It("kills the oom process", func() {
			Expect(oomNotifier.Watch(oom)).To(Succeed())

			oomNotifier.Unwatch()

			startedCommands := runner.StartedCommands()
			killedCommands := runner.KilledCommands()

			Expect(startedCommands).To(HaveLen(1))
			Expect(startedCommands).To(Equal(killedCommands))
		})
	})
})
