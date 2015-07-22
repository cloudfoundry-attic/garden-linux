package linux_container_test

import (
	"fmt"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("OomNotifier", func() {
	var (
		runner         *fake_command_runner.FakeCommandRunner
		cgroupsPath    string
		cgroupsManager linux_container.CgroupsManager
		stopCallback   func()
		containerPath  string
		oomNotifier    *linux_container.OomNotifier
	)

	BeforeEach(func() {
		runner = fake_command_runner.New()

		cgroupsPath = path.Join("path", "to", "cgroups")
		cgroupsManager = fake_cgroups_manager.New(cgroupsPath, "123456")

		stopCallback = func() {}

		containerPath = path.Join("path", "to", "container")
	})

	JustBeforeEach(func() {
		oomNotifier = linux_container.NewOomNotifier(
			runner,
			containerPath,
			stopCallback,
			cgroupsManager,
		)
	})

	Describe("Start", func() {
		It("calls oom", func() {
			Expect(oomNotifier.Start()).To(Succeed())

			Expect(runner).To(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: path.Join(containerPath, "bin", "oom"),
					Args: []string{path.Join(cgroupsPath, "memory", "instance-123456")},
				},
			))
		})

		Context("when the stop callback is set", func() {
			var stopCalled chan struct{}

			BeforeEach(func() {
				stopCalled = make(chan struct{})

				stopCallback = func() {
					fmt.Fprintf(GinkgoWriter, "Callback called!\n\n\n")
					close(stopCalled)
				}
			})

			It("calls it", func() {
				Expect(oomNotifier.Start()).To(Succeed())

				Eventually(stopCalled).Should(BeClosed())
			})
		})
	})
})
