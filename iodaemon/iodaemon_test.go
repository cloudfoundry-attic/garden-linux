package main_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry-incubator/warden-linux/ptyutil"
	"github.com/kr/pty"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Iodaemon", func() {
	It("can exhaust a single link's stdin", func() {
		spawnS, err := gexec.Start(exec.Command(
			iodaemon,
			"spawn",
			socketPath,
			"bash", "-c", "cat <&0; exit 42",
		), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		defer spawnS.Kill()

		Eventually(spawnS).Should(gbytes.Say("ready\n"))
		Consistently(spawnS).ShouldNot(gbytes.Say("pid:"))

		link := exec.Command(iodaemon, "link", socketPath)
		link.Stdin = bytes.NewBufferString("hello\ngoodbye")

		linkS, err := gexec.Start(link, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(spawnS).Should(gbytes.Say("pid:"))

		Eventually(linkS).Should(gbytes.Say("hello\ngoodbye"))
		Eventually(linkS).Should(gexec.Exit(42))
	})

	It("can read some stdin, have a link break, and exhaust more stdin", func() {
		spawnS, err := gexec.Start(exec.Command(
			iodaemon,
			"spawn",
			socketPath,
			"bash", "-c", "cat <&0; exit 42",
		), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		defer spawnS.Kill()

		Eventually(spawnS).Should(gbytes.Say("ready\n"))
		Consistently(spawnS).ShouldNot(gbytes.Say("pid:"))

		r, w, err := os.Pipe()
		Ω(err).ShouldNot(HaveOccurred())

		link := exec.Command(iodaemon, "link", socketPath)
		link.Stdin = r

		linkS, err := gexec.Start(link, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		_, err = fmt.Fprintf(w, "hello\n")
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(spawnS).Should(gbytes.Say("pid:"))

		Eventually(linkS).Should(gbytes.Say("hello\n"))
		Consistently(linkS).ShouldNot(gexec.Exit(42))

		linkS.Terminate().Wait()

		link = exec.Command(iodaemon, "link", socketPath)
		link.Stdin = r

		linkS, err = gexec.Start(link, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		_, err = fmt.Fprintf(w, "goodbye")
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(linkS).Should(gbytes.Say("goodbye"))

		err = w.Close()
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(linkS).Should(gexec.Exit(42))
	})

	Describe("spawning with -tty", func() {
		It("transports stdin, stdout, and stderr", func() {
			spawnS, err := gexec.Start(exec.Command(
				iodaemon,
				"-tty",
				"spawn",
				socketPath,
				"bash", "-c", "read foo; echo hi $foo; echo hi err >&2",
			), GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			defer spawnS.Kill()

			Eventually(spawnS).Should(gbytes.Say("ready\n"))

			inR, inW := io.Pipe()
			Ω(err).ShouldNot(HaveOccurred())

			link := exec.Command(iodaemon, "link", socketPath)
			link.Stdin = inR

			linkS, err := gexec.Start(link, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			_, err = inW.Write([]byte("out\r\n"))
			Ω(err).ShouldNot(HaveOccurred())

			err = inW.Close()
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(linkS).Should(gbytes.Say("hi out\r\n"))
			Eventually(linkS).Should(gbytes.Say("hi err\r\n"))

			Eventually(linkS).Should(gexec.Exit(0))
		})

		It("starts with an 80x24 tty, and can be resized", func() {
			spawnS, err := gexec.Start(exec.Command(
				iodaemon,
				"-tty",
				"spawn",
				socketPath,
				winsizeReporter,
			), GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			defer spawnS.Kill()

			Eventually(spawnS).Should(gbytes.Say("ready\n"))

			pty, tty, err := pty.Open()
			Ω(err).ShouldNot(HaveOccurred())

			link := exec.Command(iodaemon, "link", socketPath)
			link.Stdin = tty

			linkS, err := gexec.Start(link, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(linkS).Should(gbytes.Say("rows: 24, cols: 80\r\n"))

			err = ptyutil.SetWinSize(pty, 123, 456)
			Ω(err).ShouldNot(HaveOccurred())

			err = link.Process.Signal(syscall.SIGWINCH)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(linkS).Should(gbytes.Say("rows: 456, cols: 123\r\n"))
			Eventually(linkS).Should(gexec.Exit(0))
		})
	})
})
