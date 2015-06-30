package unix_socket_test

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket/fake_connection_handler"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"fmt"
	"net"
)

var _ = Describe("Unix socket", func() {
	var (
		listener          *unix_socket.Listener
		connector         *unix_socket.Connector
		connectionHandler *fake_connection_handler.FakeConnectionHandler
		socketPath        string
		sentPid           int
		sentError         error
		sentErrorMutex    sync.Mutex
	)

	BeforeEach(func() {
		tmpDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		socketPath = path.Join(tmpDir, "the_socket_file.sock")

		sentErrorMutex.Lock()
		defer sentErrorMutex.Unlock()
		sentError = nil
		sentPid = 0
		connectionHandler = &fake_connection_handler.FakeConnectionHandler{}
	})

	JustBeforeEach(func() {
		connector = &unix_socket.Connector{
			SocketPath: socketPath,
		}
	})

	Describe("Listener creation", func() {
		Context("when listener is created by socket path", func() {
			Context("when file does not exist", func() {
				Context("when there is no permission to create the file", func() {
					It("returns an error", func() {
						socketPath := "/proc/a_socket.sock"
						_, err := unix_socket.NewListenerFromPath(socketPath)
						Expect(err).To(HaveOccurred())
					})
				})

				Context("when there is permission to create the file", func() {
					It("creates the listener", func() {
						socketPath := fmt.Sprintf("/tmp/a_socket-%d.sock", GinkgoParallelNode())

						listener, err := unix_socket.NewListenerFromPath(socketPath)
						Expect(err).ToNot(HaveOccurred())

						Expect(listener.Close()).To(Succeed())
					})
				})
			})

			Context("when the file does exist", func() {
				It("returns an error", func() {
					socketFile, err := ioutil.TempFile("", "")
					Expect(err).ToNot(HaveOccurred())

					_, err = unix_socket.NewListenerFromPath(socketFile.Name())
					Expect(err).To(HaveOccurred())

					socketFile.Close()
				})
			})
		})

		Context("when listener is created by socket file", func() {
			Context("when the file does exist", func() {
				Context("when the file is not a socket file", func() {
					It("returns an error", func() {
						socketFile, err := ioutil.TempFile("", "")
						Expect(err).ToNot(HaveOccurred())

						_, err = unix_socket.NewListenerFromPath(socketFile.Name())
						Expect(err).To(HaveOccurred())

						socketFile.Close()
					})
				})

				Context("when the file is a socket file", func() {
					It("creates the listener", func() {
						socketPath := fmt.Sprintf("/tmp/a_socket-%d.sock", GinkgoParallelNode())
						socketListener, err := net.Listen("unix", socketPath)
						Expect(err).ToNot(HaveOccurred())

						socketFile, err := socketListener.(*net.UnixListener).File()
						Expect(err).ToNot(HaveOccurred())

						listener, err := unix_socket.NewListenerFromFile(socketFile)
						Expect(err).ToNot(HaveOccurred())

						Expect(socketFile.Close()).To(Succeed())
						Expect(listener.Close()).To(Succeed())
					})
				})
			})
		})
	})

	Describe("Connect", func() {
		Context("when the server is not running", func() {
			It("fails to connect", func() {
				_, _, err := connector.Connect(nil)
				Expect(err).To(MatchError(ContainSubstring("unix_socket: connect to server socket")))
			})
		})

		Context("when the server is running", func() {
			var recvMsg map[string]string
			var sentFiles []*os.File
			var stubDone chan bool

			JustBeforeEach(func() {
				var err error
				listener, err = unix_socket.NewListenerFromPath(socketPath)
				Expect(err).NotTo(HaveOccurred())

				f1r, f1w, err := os.Pipe()
				Expect(err).ToNot(HaveOccurred())
				f2r, f2w, err := os.Pipe()
				Expect(err).ToNot(HaveOccurred())
				sentFiles = []*os.File{f1r, f2w}

				sentPid = 123

				stubDone = make(chan bool, 1)

				sentFilesCp := []*os.File{f1w, f2r}
				sentErrorMutex.Lock()
				defer sentErrorMutex.Unlock()
				sentErrorCp := sentError
				sentPidCp := sentPid
				connectionHandler.HandleStub = func(decoder *json.Decoder) ([]*os.File, int, error) {
					defer GinkgoRecover()
					err := decoder.Decode(&recvMsg)
					Expect(err).ToNot(HaveOccurred())
					stubDone <- true

					return sentFilesCp, sentPidCp, sentErrorCp
				}

				go listener.Listen(connectionHandler)
			})

			AfterEach(func() {
				if listener != nil {
					Expect(listener.Close()).To(Succeed())
				}
			})

			It("calls the handler with the sent message", func() {
				sentMsg := map[string]string{"fruit": "apple"}
				_, _, err := connector.Connect(sentMsg)
				Expect(err).ToNot(HaveOccurred())

				Eventually(stubDone).Should(Receive())
				Expect(recvMsg).To(Equal(sentMsg))
			})

			It("gets back the stream the handler provided", func() {
				sentMsg := map[string]string{"fruit": "apple"}
				streams, _, err := connector.Connect(sentMsg)
				Expect(err).ToNot(HaveOccurred())

				Expect(stubDone).To(Receive())
				Expect(streams).To(HaveLen(2))

				_, err = streams[0].Write([]byte("potato potato"))
				Expect(err).NotTo(HaveOccurred())
				err = streams[0].Close()
				Expect(err).NotTo(HaveOccurred())
				sentFiles[0].Seek(0, 0)
				Expect(ioutil.ReadAll(sentFiles[0])).Should(Equal([]byte("potato potato")))

				_, err = sentFiles[1].Write([]byte("brocoli brocoli"))
				Expect(err).NotTo(HaveOccurred())
				err = sentFiles[1].Close()
				Expect(err).NotTo(HaveOccurred())
				Expect(ioutil.ReadAll(streams[1])).Should(Equal([]byte("brocoli brocoli")))
			})

			It("gets back the pid the handler provided", func() {
				sentMsg := map[string]string{"fruit": "apple"}
				_, pid, err := connector.Connect(sentMsg)
				Expect(err).ToNot(HaveOccurred())
				Eventually(stubDone).Should(Receive())
				Expect(pid).To(Equal(sentPid))
			})

			Context("when the handler fails", func() {
				BeforeEach(func() {
					sentErrorMutex.Lock()
					defer sentErrorMutex.Unlock()
					sentError = errors.New("no cake")
				})

				It("sends back the error from the handler", func() {
					sentMsg := map[string]string{"fruit": "apple"}
					_, _, err := connector.Connect(sentMsg)
					Expect(err).To(MatchError("no cake"))
				})
			})
		})
	})
})
