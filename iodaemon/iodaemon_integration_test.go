package main_test

import (
	"os"
	"os/exec"

	linkpkg "github.com/cloudfoundry-incubator/garden-linux/iodaemon/link"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Iodaemon integration tests", func() {
	It("can read stdin", func() {
		spawnS, err := gexec.Start(exec.Command(
			iodaemon,
			"spawn",
			socketPath,
			"bash", "-c", "cat <&0; exit 42",
		), GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		defer spawnS.Kill()

		Eventually(spawnS).Should(gbytes.Say("ready\n"))
		Consistently(spawnS).ShouldNot(gbytes.Say("active\n"))

		linkStdout := gbytes.NewBuffer()
		link, err := linkpkg.Create(socketPath, linkStdout, os.Stderr)
		Expect(err).ToNot(HaveOccurred())

		link.Write([]byte("hello\ngoodbye"))
		link.Close()

		Eventually(spawnS).Should(gbytes.Say("active\n"))
		Eventually(linkStdout).Should(gbytes.Say("hello\ngoodbye"))

		Expect(link.Wait()).To(Equal(42))
	})

	It("can read stdin in tty mode", func() {
		spawnS, err := gexec.Start(exec.Command(
			iodaemon,
			"-tty",
			"spawn",
			socketPath,
			"bash", "-c", "cat <&0; exit 42",
		), GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		defer spawnS.Kill()

		Eventually(spawnS).Should(gbytes.Say("ready\n"))
		Consistently(spawnS).ShouldNot(gbytes.Say("active\n"))

		linkStdout := gbytes.NewBuffer()
		link, err := linkpkg.Create(socketPath, linkStdout, os.Stderr)
		Expect(err).ToNot(HaveOccurred())

		link.Write([]byte("hello\ngoodbye"))
		link.Close()

		Eventually(spawnS).Should(gbytes.Say("active\n"))
		Eventually(linkStdout).Should(gbytes.Say("hello\r\ngoodbye"))

		Expect(link.Wait()).To(Equal(-1)) // -1 indicates unhandled SIGHUP
	})

	It("consistently executes a quickly-printing-and-exiting command", func() {
		for i := 0; i < 100; i++ {
			spawnS, err := gexec.Start(exec.Command(
				iodaemon,
				"spawn",
				socketPath,
				"echo", "hi",
			), GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(spawnS).Should(gbytes.Say("ready\n"))

			lk, err := linkpkg.Create(socketPath, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			lk.Close()

			Eventually(spawnS).Should(gbytes.Say("active\n"))
			Eventually(spawnS).Should(gexec.Exit(0))
		}
	})

	It("times out while spawning when no listeners connect", func() {
		process, err := gexec.Start(exec.Command(
			iodaemon,
			"-timeout 5s",
			"spawn",
			socketPath,
			"bash", "-c", "cat <&0",
		), GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		Eventually(process, "6s").Should(gexec.Exit(2))
	})
})
