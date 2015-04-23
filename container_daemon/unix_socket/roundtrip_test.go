package unix_socket_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket/fake_connection_handler"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Unix socket", func() {
	var (
		listener          *unix_socket.Listener
		connector         *unix_socket.Connector
		connectionHandler *fake_connection_handler.FakeConnectionHandler
		socketPath        string
	)

	BeforeEach(func() {
		tmpDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		socketPath = path.Join(tmpDir, "the_socket_file.sock")
	})

	JustBeforeEach(func() {
		connectionHandler = &fake_connection_handler.FakeConnectionHandler{}

		connector = &unix_socket.Connector{
			SocketPath: socketPath,
		}

		listener = &unix_socket.Listener{
			SocketPath: socketPath,
		}
	})

	Describe("Listener.Init", func() {
		It("creates a unix socket for the given socket path", func() {
			err := listener.Init()
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
				err := listener.Init()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	PDescribe("Listener.Stop", func() {})

	Describe("Connect", func() {
		Context("when the server is not running", func() {
			It("fails to connect", func() {
				_, err := connector.Connect(nil)
				Expect(err).To(MatchError(ContainSubstring("unix_socket: connect to server socket")))
			})
		})

		FContext("when the server is running", func() {
			var recvMsg map[string]string
			var sentFiles []*os.File
			var stubDone chan bool

			JustBeforeEach(func() {
				Expect(listener.Init()).To(Succeed())

				f1, _ := ioutil.TempFile("", "")
				f2, _ := ioutil.TempFile("", "")
				sentFiles = []*os.File{f1, f2}

				stubDone = make(chan bool, 1)

				connectionHandler.HandleStub = func(decoder *json.Decoder) ([]*os.File, error) {
					defer GinkgoRecover()
					err := decoder.Decode(&recvMsg)
					Expect(err).ToNot(HaveOccurred())
					stubDone <- true

					return sentFiles, nil
				}

				go listener.Listen(connectionHandler)
			})

			AfterEach(func() {
				Expect(listener.Stop()).To(Succeed())
			})

			It("calls the handler with the sent message", func() {
				sentMsg := map[string]string{"fruit": "apple"}
				_, err := connector.Connect(sentMsg)
				Expect(err).ToNot(HaveOccurred())

				Eventually(stubDone).Should(Receive())
				Expect(recvMsg).To(Equal(sentMsg))
			})

			FIt("gets back the stream the handler provided", func() {
				sentMsg := map[string]string{"fruit": "apple"}
				streams, err := connector.Connect(sentMsg)
				Expect(err).ToNot(HaveOccurred())

				Expect(stubDone).To(Receive())
				Expect(streams).To(HaveLen(2))

				_, err = streams[0].Write([]byte("potato potato"))
				Expect(err).NotTo(HaveOccurred())
				sentFiles[0].Seek(0, 0)
				Expect(ioutil.ReadAll(sentFiles[0])).Should(Equal([]byte("potato potato")))

				_, err = sentFiles[1].Write([]byte("brocoli brocoli"))
				Expect(err).NotTo(HaveOccurred())
				sentFiles[1].Seek(0, 0)
				Expect(ioutil.ReadAll(streams[1])).Should(Equal([]byte("brocoli brocoli")))
			})

			PContext("when the handler fails", func() {})
		})
	})

	Describe("Listener.Run", func() {
		Context("when the listener is not initialized", func() {
			It("returns an error", func() {
				err := listener.Listen(connectionHandler)
				Expect(err).To(MatchError("unix_socket: listener is not initialized"))
			})
		})
	})
})
