package container_daemon_test

import (
	"io/ioutil"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("proc_starter", func() {
	It("runs the process in the specified working directory", func() {
		testWorkDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		cmd := exec.Command(procStarterBin, "ENCODEDRLIMITS=", "/bin/sh", "-c", "echo $PWD")
		cmd.Dir = testWorkDir
		op, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred())
		Expect(string(op)).To(Equal(testWorkDir + "\n"))
	})

	It("runs a program from the PATH", func() {
		cmd := exec.Command(procStarterBin, "ENCODEDRLIMITS=", "ls", "/")
		Expect(cmd.Run()).To(Succeed())
	})

	It("closes any open FDs before starting the process", func() {
		file, err := os.Open("/dev/zero")
		Expect(err).NotTo(HaveOccurred())

		pipe, _, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		cmd := exec.Command(procStarterBin, "ENCODEDRLIMITS=", "ls", "/proc/self/fd")
		cmd.ExtraFiles = []*os.File{
			file,
			pipe,
		}

		session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		Eventually(session).Should(gexec.Exit(0))
		Expect(session.Out.Contents()).To(Equal([]byte("0\n1\n2\n3\n"))) // stdin, stdout, stderr, /proc/self/fd
	})
})
