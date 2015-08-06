package container_daemon_test

import (
	"fmt"
	"io"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Signalling a running process", func() {
	var pid int
	var signaller *container_daemon.ProcessSignaller
	var stdout *gbytes.Buffer

	BeforeEach(func() {
		stdout = gbytes.NewBuffer()
		cmd := exec.Command("bash", "-c", `
		trap "echo TERMed; exit" TERM
		echo "pid = $$"
		sleep 2
	`)
		cmd.Stdout = io.MultiWriter(stdout, GinkgoWriter)
		cmd.Stderr = GinkgoWriter

		err := cmd.Start()
		Expect(err).NotTo(HaveOccurred())

		Eventually(stdout).Should(gbytes.Say("pid"))
		_, err = fmt.Sscanf(string(stdout.Contents()), "pid = %d\n", &pid)
		Expect(err).ToNot(HaveOccurred())

		signaller = &container_daemon.ProcessSignaller{
			Logger: lagertest.NewTestLogger("test"),
		}
	})

	Context("when a process with the given pid exists", func() {
		It("sends the signal to the process", func() {
			Expect(signaller.Signal(pid, syscall.SIGTERM)).To(Succeed())
			Eventually(stdout, "5s").Should(gbytes.Say("TERMed"))
		})
	})

	Context("when a process with the given pid does not exist", func() {
		It("returns an error", func() {
			err := signaller.Signal(123123123, syscall.SIGTERM)
			Expect(err).To(MatchError(ContainSubstring("container_daemon: signaller: signal process: pid:")))
		})
	})
})
