package link_test

import (
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"

	linkpkg "code.cloudfoundry.org/garden-linux/iodaemon/link"
	"code.cloudfoundry.org/garden-linux/iodaemon/link/fake_unix_server"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Link", func() {
	var (
		unixSockerPath            string
		fakeServer                *fake_unix_server.FakeUnixServer
		stdout, stderr            *gbytes.Buffer
		stdoutW, stderrW, statusW *os.File
	)

	BeforeEach(func() {
		tmpDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		unixSockerPath = path.Join(tmpDir, "iodaemon.sock")

		fakeServer, err = fake_unix_server.NewFakeUnixServer(unixSockerPath)
		Expect(err).ToNot(HaveOccurred())

		stdout = gbytes.NewBuffer()
		stderr = gbytes.NewBuffer()

		var (
			stdoutR, stderrR, statusR *os.File
		)

		stdoutR, stdoutW, err = os.Pipe()
		Expect(err).ToNot(HaveOccurred())
		stderrR, stderrW, err = os.Pipe()
		Expect(err).ToNot(HaveOccurred())
		statusR, statusW, err = os.Pipe()
		Expect(err).ToNot(HaveOccurred())

		fakeServer.SetConnectionHandler(func(conn net.Conn) {
			rights := syscall.UnixRights(
				int(stdoutR.Fd()),
				int(stderrR.Fd()),
				int(statusR.Fd()),
			)

			conn.(*net.UnixConn).WriteMsgUnix([]byte{}, rights, nil)
		})
	})

	JustBeforeEach(func() {
		go fakeServer.Serve()
	})

	AfterEach(func() {
		Expect(fakeServer.Stop()).To(Succeed())

		Expect(os.RemoveAll(path.Base(unixSockerPath))).To(Succeed())
	})

	Describe("Create", func() {
		Context("when files are not provided", func() {
			BeforeEach(func() {
				fakeServer.SetConnectionHandler(func(conn net.Conn) {
					conn.Close()
				})
			})

			It("returns an error", func() {
				_, err := linkpkg.Create(unixSockerPath, stdout, stderr)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when files are provided", func() {
			AfterEach(func() {
				stdoutW.Close()
				stderrW.Close()
				statusW.Close()
			})

			It("succeeds", func() {
				_, err := linkpkg.Create(unixSockerPath, stdout, stderr)
				Expect(err).ToNot(HaveOccurred())
			})

			It("streams stdout", func() {
				_, err := linkpkg.Create(unixSockerPath, stdout, stderr)
				Expect(err).ToNot(HaveOccurred())

				stdoutW.Write([]byte("Hello stdout banana"))
				Eventually(stdout).Should(gbytes.Say("Hello stdout banana"))
			})

			It("streams stderr", func() {
				_, err := linkpkg.Create(unixSockerPath, stdout, stderr)
				Expect(err).ToNot(HaveOccurred())

				stderrW.Write([]byte("Hello stderr banana"))
				Eventually(stderr).Should(gbytes.Say("Hello stderr banana"))
			})

			It("should set close on exec for all new file descriptors", func() {
				initialNumFdsWithoutCloseOnExec := numFdsWithoutCloseOnExec()
				_, err := linkpkg.Create(unixSockerPath, stdout, stderr)
				Expect(err).ToNot(HaveOccurred())

				finalNumFdsWithoutCloseOnExec := numFdsWithoutCloseOnExec()
				Expect(finalNumFdsWithoutCloseOnExec).To(Equal(initialNumFdsWithoutCloseOnExec))
			})
		})
	})
})

func numFdsWithoutCloseOnExec() int {
	sleepCmd := exec.Command("sleep", "1")
	Expect(sleepCmd.Start()).To(Succeed())
	pid := sleepCmd.Process.Pid

	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid)).Output()
	Expect(err).ToNot(HaveOccurred())

	return strings.Count(string(out), "\n")
}
