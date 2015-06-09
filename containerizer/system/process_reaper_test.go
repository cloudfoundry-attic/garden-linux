package system_test

import (
	"fmt"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/Godeps/_workspace/src/github.com/onsi/gomega/gbytes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProcessReaper", func() {
	var reaper *system.ProcessReaper

	BeforeEach(func() {
		logger := lager.NewLogger("process_reaper_test_logger")
		logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.ERROR))
		reaper = system.StartReaper(logger)
	})

	AfterEach(func() {
		reaper.Stop()
	})

	It("waits for a process to return and returns its exit status", func() {
		cmd := exec.Command("sh", "-c", "exit 3")
		Expect(reaper.Start(cmd)).To(Succeed())

		Expect(reaper.Wait(cmd)).To(Equal(byte(3)))
	})

	It("waits for multiple processes", func() {
		cmd1 := exec.Command("sh", "-c", "exit 3")
		cmd2 := exec.Command("sh", "-c", "exit 33")

		Expect(reaper.Start(cmd1)).To(Succeed())
		Expect(reaper.Start(cmd2)).To(Succeed())

		Expect(reaper.Wait(cmd1)).To(Equal(byte(3)))
		Expect(reaper.Wait(cmd2)).To(Equal(byte(33)))
	})

	Context("when there are grandchildren processes", func() {
		It("waits for a process to return and returns its exit status", func() {
			cmd := exec.Command("sh", "-c", "sleep 1; exit 3")
			Expect(reaper.Start(cmd)).To(Succeed())
			Expect(reaper.Wait(cmd)).To(Equal(byte(3)))
		})

		It("the child process can receive SIGCHLD when a grandchild terminates", func() {
			stdout := gbytes.NewBuffer()
			trap := exec.Command("sh", "-c", "trap 'echo caught SIGCHLD' CHLD; (ls / >/dev/null 2/&1); exit 0")
			trap.Stdout = stdout

			Expect(reaper.Start(trap)).To(Succeed())
			Expect(reaper.Wait(trap)).To(Equal(byte(0)))
			Expect(stdout).To(gbytes.Say("caught SIGCHLD\n"))
		})
	})

	It("returns correct exit statuses of short-lived processes", func(done Done) {
		for i := 0; i < 100; i++ {
			cmd := exec.Command("sh", "-c", "exit 42")
			Expect(reaper.Start(cmd)).To(Succeed())

			cmd2 := exec.Command("sh", "-c", "exit 43")
			Expect(reaper.Start(cmd2)).To(Succeed())

			cmd3 := exec.Command("sh", "-c", "exit 44")
			Expect(reaper.Start(cmd3)).To(Succeed())

			exitStatus := reaper.Wait(cmd3)
			Expect(exitStatus).To(Equal(byte(44)))

			exitStatus = reaper.Wait(cmd2)
			Expect(exitStatus).To(Equal(byte(43)))

			exitStatus = reaper.Wait(cmd)
			Expect(exitStatus).To(Equal(byte(42)))
		}
		close(done)
	}, 90.0)

	It("reaps processes when they terminate in close succession", func(done Done) {
		for i := 0; i < 100; i++ {
			cmd := exec.Command("sh", "-c", `while true; do sleep 1; done`)
			Expect(reaper.Start(cmd)).To(Succeed())

			kill := exec.Command("kill", "-9", fmt.Sprintf("%d", cmd.Process.Pid))
			Expect(reaper.Start(kill)).To(Succeed())

			exitStatus := reaper.Wait(kill)
			Expect(exitStatus).To(Equal(byte(0)))

			exitStatus = reaper.Wait(cmd)
			Expect(exitStatus).To(Equal(byte(255)))
		}
		close(done)
	}, 90.0)
})
