package system_test

import (
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/pivotal-golang/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("ProcessReaper", func() {
	var reaper *system.ProcessReaper

	BeforeEach(func() {
		reaper = system.StartReaper(lager.NewLogger("process_reaper_test_logger"))
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
	})
})
