package networking_test

import (
	"fmt"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Networking recovery", func() {
	PContext("with two containers in the same subnet", func() {
		var (
			ctr1           garden.Container
			ctr2           garden.Container
			bridgeEvidence string
		)
		BeforeEach(func() {
			client = startGarden()

			containerNetwork := fmt.Sprintf("10.%d.0.0/24", GinkgoParallelNode())
			var err error
			ctr1, err = client.Create(garden.ContainerSpec{Network: containerNetwork})
			Expect(err).ToNot(HaveOccurred())
			ctr2, err = client.Create(garden.ContainerSpec{Network: containerNetwork})
			Expect(err).ToNot(HaveOccurred())

			bridgeEvidence = fmt.Sprintf("inet 10.%d.0.254/24 scope global w%db-", GinkgoParallelNode(), GinkgoParallelNode())
			cmd := exec.Command("ip", "a")
			Expect(cmd.CombinedOutput()).To(ContainSubstring(bridgeEvidence))
		})

		Context("when garden is killed and restarted using SIGKILL", func() {
			BeforeEach(func() {
				gardenProcess.Signal(syscall.SIGKILL)
				Eventually(gardenProcess.Wait(), "10s").Should(Receive())

				client = startGarden()
				Expect(client.Ping()).ToNot(HaveOccurred())
			})

			It("the subnet's bridge no longer exists", func() {
				cmd := exec.Command("ip", "a")
				Expect(cmd.CombinedOutput()).ToNot(ContainSubstring(bridgeEvidence))
			})
		})

		Context("when garden is shut down cleanly and restarted, and the containers are deleted", func() {
			BeforeEach(func() {
				gardenProcess.Signal(syscall.SIGTERM)
				Eventually(gardenProcess.Wait(), "10s").Should(Receive())

				client = startGarden()
				Expect(client.Ping()).ToNot(HaveOccurred())

				cmd := exec.Command("ip", "a")
				Expect(cmd.CombinedOutput()).To(ContainSubstring(bridgeEvidence))

				Expect(client.Destroy(ctr1.Handle())).To(Succeed())
				Expect(client.Destroy(ctr2.Handle())).To(Succeed())
			})

			It("the subnet's bridge no longer exists", func() {
				cmd := exec.Command("ip", "a")
				Expect(cmd.CombinedOutput()).ToNot(ContainSubstring(bridgeEvidence))
			})
		})

		Context("when garden is shut down and restarted", func() {
			BeforeEach(func() {
				gardenProcess.Signal(syscall.SIGTERM)
				Eventually(gardenProcess.Wait(), "10s").Should(Receive())

				client = startGarden()
				Expect(client.Ping()).ToNot(HaveOccurred())
			})

			It("the subnet's bridge still exists", func() {
				cmd := exec.Command("ip", "a")
				Expect(cmd.CombinedOutput()).To(ContainSubstring(bridgeEvidence))
			})

			It("containers are still pingable", func() {
				info1, ierr := ctr1.Info()
				Expect(ierr).ToNot(HaveOccurred())

				out, err := exec.Command("/bin/ping", "-c 2", info1.ContainerIP).Output()
				Expect(out).To(ContainSubstring(" 0% packet loss"))
				Expect(err).ToNot(HaveOccurred())
			})

			It("a container can still reach external networks", func() {
				Expect(checkInternet(ctr1)).To(Succeed())
			})
		})
	})

})
