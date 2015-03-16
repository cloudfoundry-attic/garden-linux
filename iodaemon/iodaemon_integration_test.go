package main_test

import (
	"os"
	"os/exec"
	"syscall"

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
		Ω(err).ShouldNot(HaveOccurred())

		defer spawnS.Kill()

		Eventually(spawnS).Should(gbytes.Say("ready\n"))
		Consistently(spawnS).ShouldNot(gbytes.Say("active\n"))

		linkStdout := gbytes.NewBuffer()
		link, err := linkpkg.Create(socketPath, linkStdout, os.Stderr)
		Ω(err).ShouldNot(HaveOccurred())

		link.Write([]byte("hello\ngoodbye"))
		link.Close()

		Eventually(spawnS).Should(gbytes.Say("active\n"))
		Eventually(linkStdout).Should(gbytes.Say("hello\ngoodbye"))

		Ω(link.Wait()).Should(Equal(42))
	})

	It("can read stdin in tty mode", func() {
		spawnS, err := gexec.Start(exec.Command(
			iodaemon,
			"-tty",
			"spawn",
			socketPath,
			"bash", "-c", "cat <&0; exit 42",
		), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		defer spawnS.Kill()

		Eventually(spawnS).Should(gbytes.Say("ready\n"))
		Consistently(spawnS).ShouldNot(gbytes.Say("active\n"))

		linkStdout := gbytes.NewBuffer()
		link, err := linkpkg.Create(socketPath, linkStdout, os.Stderr)
		Ω(err).ShouldNot(HaveOccurred())

		link.Write([]byte("hello\ngoodbye"))
		link.Close()

		Eventually(spawnS).Should(gbytes.Say("active\n"))
		Eventually(linkStdout).Should(gbytes.Say("hello\r\ngoodbye"))

		Ω(link.Wait()).Should(Equal(-1)) // -1 indicates unhandled SIGHUP
	})

	It("consistently executes a quickly-printing-and-exiting command", func() {
		for i := 0; i < 100; i++ {
			spawnS, err := gexec.Start(exec.Command(
				iodaemon,
				"spawn",
				socketPath,
				"echo", "hi",
			), GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(spawnS).Should(gbytes.Say("ready\n"))

			lk, err := linkpkg.Create(socketPath, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())
			lk.Close()

			Eventually(spawnS).Should(gbytes.Say("active\n"))
			Eventually(spawnS).Should(gexec.Exit(0))
		}
	})

	It("can be killed via a signal", func() {
		spawnS, err := gexec.Start(exec.Command(
			iodaemon,
			"spawn",
			socketPath,
			"bash",
		), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(spawnS).Should(gbytes.Say("ready\n"))

		lk, err := linkpkg.Create(socketPath, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		lk.Signal(syscall.SIGTERM)
		processExitCode, err := lk.Wait()
		Ω(err).ShouldNot(HaveOccurred())

		Ω(processExitCode).ShouldNot(Equal(0))

		Eventually(spawnS).Should(gbytes.Say("active\n"))
		Eventually(spawnS).Should(gexec.Exit(0))
	})
})
