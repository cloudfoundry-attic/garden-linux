package container_daemon_test

import (
	"io/ioutil"
	"os"
	"os/exec"

	"io"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("proc_starter", func() {

	var procStarterBin string

	BeforeEach(func() {
		var err error
		procStarterBin, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/container_daemon/proc_starter")
		Expect(err).ToNot(HaveOccurred())
	})

	It("runs the process in the specified working directory", func() {
		testWorkDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		cmd := exec.Command(procStarterBin, "-uid=0", "-gid=0", "--", "/bin/sh", "-c", "echo $PWD")
		cmd.Dir = testWorkDir
		op, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred())
		Expect(string(op)).To(Equal(testWorkDir + "\n"))
	})

	It("runs a program from the PATH", func() {
		cmd := exec.Command(procStarterBin, "-uid=0", "-gid=0", "--", "ls", "/")
		Expect(cmd.Run()).To(Succeed())
	})

	It("sets rlimits", func() {
		cmd := exec.Command(procStarterBin, "-uid=0", "-gid=0", "-rlimits=RLIMIT_NOFILE=2099,RLIMIT_CPU=3", "--", "sh", "-c", "ulimit -a")
		out := gbytes.NewBuffer()
		cmd.Stdout = io.MultiWriter(GinkgoWriter, out)
		cmd.Stderr = GinkgoWriter

		Expect(cmd.Run()).To(Succeed())
		Expect(out).To(gbytes.Say("nofiles\\s+2099"))
	})

	It("allows the spawned process to have its own args", func() {
		cmd := exec.Command(procStarterBin, "-uid=0", "-gid=0", "-rlimits=", "-dropCapabilities=false", "--", "echo", "foo", "-bar", "-baz=beans")
		out := gbytes.NewBuffer()
		cmd.Stdout = io.MultiWriter(GinkgoWriter, out)
		cmd.Stderr = GinkgoWriter

		Expect(cmd.Run()).To(Succeed())
		Expect(out).To(gbytes.Say("foo -bar -baz=beans"))
	})

	It("drops capabilities before starting the process", func() {
		cmd := exec.Command(procStarterBin, "-uid=0", "-gid=0", "--", "cat", "/proc/self/status")
		out := gbytes.NewBuffer()
		cmd.Stdout = io.MultiWriter(GinkgoWriter, out)
		cmd.Stderr = io.MultiWriter(GinkgoWriter, out)
		Expect(cmd.Run()).To(Succeed())
		Expect(out).To(gbytes.Say("CapBnd:	00000000a80425fb"))
	})

	Context("when the dropCapabilities flag is set to false", func() {
		It("does not drop capabilties before starting the process", func() {
			cmd := exec.Command(procStarterBin, "-uid=0", "-gid=0", "-dropCapabilities=false", "--", "cat", "/proc/self/status")
			out := gbytes.NewBuffer()
			cmd.Stdout = io.MultiWriter(GinkgoWriter, out)
			cmd.Stderr = io.MultiWriter(GinkgoWriter, out)
			Expect(cmd.Run()).To(Succeed())
			Expect(out).ToNot(gbytes.Say("CapBnd:	0000000000000000"))
		})
	})

	It("closes any open FDs before starting the process", func() {
		file, err := os.Open("/dev/zero")
		Expect(err).NotTo(HaveOccurred())

		pipe, _, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		cmd := exec.Command(procStarterBin, "-uid=0", "-gid=0", "--", "ls", "/proc/self/fd")
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
