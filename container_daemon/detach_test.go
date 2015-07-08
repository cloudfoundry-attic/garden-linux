package container_daemon_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Detach", func() {
	var lsof *gexec.Session
	var hostDir string
	var detachCmd *exec.Cmd

	var stderrRedirect string
	var stdoutRedirect string

	BeforeEach(func() {
		var err error
		hostDir, err = ioutil.TempDir("", "hostdir")
		Expect(err).NotTo(HaveOccurred())

		detacher, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/container_daemon/detach_test")
		Expect(err).NotTo(HaveOccurred())

		containerDir, err := ioutil.TempDir("", "containerdir")
		Expect(err).NotTo(HaveOccurred())

		stdoutRedirect = path.Join(containerDir, "stdout-redirect")
		stderrRedirect = path.Join(containerDir, "stderr-redirect")
		detachCmd = exec.Command(detacher, stdoutRedirect, stderrRedirect)
		detachCmd.Dir = hostDir
	})

	AfterEach(func() {
		Expect(os.RemoveAll(hostDir)).To(Succeed())
	})

	JustBeforeEach(func() {
		session, err := gexec.Start(detachCmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		Eventually(stdoutRedirect).Should(BeAnExistingFile())

		defer session.Kill()

		lsof, err = gexec.Start(exec.Command("lsof", "-n", "-p", fmt.Sprintf("%d", detachCmd.Process.Pid)), GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		Eventually(lsof).Should(gexec.Exit(0))
	})

	Describe("to avoid keeping the original rootfs busy", func() {
		It("changes working directory", func() {
			Expect(lsof).NotTo(gbytes.Say(hostDir))
		})

		Context("when stdin is open on the original rootfs", func() {
			BeforeEach(func() {
				var err error
				detachCmd.Stdin, err = os.OpenFile(path.Join(hostDir, "original-stdin"), os.O_CREATE, 0700)
				Expect(err).NotTo(HaveOccurred())
			})

			It("detaches from stdin", func() {
				Expect(lsof).NotTo(gbytes.Say(hostDir))
			})
		})

		Context("when stderr is open on the original rootfs", func() {
			BeforeEach(func() {
				var err error
				detachCmd.Stderr, err = os.OpenFile(path.Join(hostDir, "original-stderr"), os.O_CREATE|os.O_RDWR, 0700)
				Expect(err).NotTo(HaveOccurred())
			})

			It("redirects stderr to the requested file", func() {
				Expect(lsof).To(gbytes.Say("2w\\W+REG.*%s\n", stderrRedirect))
			})
		})

		Context("when stdout is open on the original rootfs", func() {
			BeforeEach(func() {
				var err error
				detachCmd.Stdout, err = os.OpenFile(path.Join(hostDir, "original-stdout"), os.O_CREATE|os.O_RDWR, 0700)
				Expect(err).NotTo(HaveOccurred())
			})

			It("redirects stdout to the requested file", func() {
				Expect(lsof).To(gbytes.Say("1w\\W+REG.*%s\n", stdoutRedirect))
			})
		})
	})
})
