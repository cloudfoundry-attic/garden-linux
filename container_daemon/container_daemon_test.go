package container_daemon_test

import (
	"io/ioutil"
	"net"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Daemon", func() {
	var (
		daemon     container_daemon.ContainerDaemon
		socketPath string
	)

	BeforeEach(func() {
		tmpDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		socketPath = path.Join(tmpDir, "the_socket_file.sock")
	})

	JustBeforeEach(func() {
		daemon = container_daemon.ContainerDaemon{
			SocketPath: socketPath,
		}
	})

	Describe("Init", func() {
		It("creates a unix socket for the given socket path", func() {
			err := daemon.Init()
			Expect(err).ToNot(HaveOccurred())

			stat, err := os.Stat(socketPath)
			Expect(err).ToNot(HaveOccurred())

			Expect(stat.Mode() & os.ModeSocket).ToNot(Equal(0))
		})

		Context("when the socket cannot be created", func() {
			BeforeEach(func() {
				socketPath = "somewhere/that/does/not/exist"
			})

			It("returns an error", func() {
				err := daemon.Init()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Run", func() {
		Context("after initialization", func() {
			JustBeforeEach(func() {
				Expect(daemon.Init()).To(Succeed())
				go daemon.Run()
			})

			AfterEach(func() {
				Expect(daemon.Stop()).To(Succeed())
			})

			It("accepts connections on its socket", func() {
				conn, err := net.Dial("unix", socketPath)
				Expect(err).ToNot(HaveOccurred())
				defer conn.Close()

				_, err = conn.Write([]byte("booo"))
				Expect(err).ToNot(HaveOccurred())

				buffer := make([]byte, 1024)
				_, err = conn.Read(buffer)
				Expect(err).ToNot(HaveOccurred())

				Expect(string(buffer)).To(ContainSubstring("Accepting connections"))
			})
		})

		Context("before initialization", func() {
			PIt("returns an error", func() {
			})
		})
	})
})
