package unix_socket_test

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"sync"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_connection_handler"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
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
				_, err := connector.Connect(nil)
				Expect(err).To(MatchError(ContainSubstring("unix_socket: connect to server socket")))
			})
		})

		Context("when the server is running", func() {
			var recvMsg *container_daemon.RequestMessage
			var sentFiles []*os.File
			var stubDone chan bool
			var sentMsg *container_daemon.RequestMessage

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

				sentFilesCp := []container_daemon.StreamingFile{f1w, f2r}
				sentErrorMutex.Lock()
				defer sentErrorMutex.Unlock()
				sentErrorCp := sentError
				sentPidCp := sentPid
				connectionHandler.HandleStub = func(decoder *json.Decoder) (*container_daemon.ResponseMessage, error) {
					defer GinkgoRecover()
					err := decoder.Decode(&recvMsg)
					Expect(err).ToNot(HaveOccurred())
					stubDone <- true

					resp := &container_daemon.ResponseMessage{
						Files: sentFilesCp,
						Pid:   sentPidCp,
					}
					if sentErrorCp != nil {
						resp.ErrMessage = sentErrorCp.Error()
					}
					return resp, sentErrorCp
				}

				go listener.Listen(connectionHandler)
			})

			AfterEach(func() {
				if listener != nil {
					Expect(listener.Close()).To(Succeed())
				}
			})

			BeforeEach(func() {
				data := map[string]string{"fruit": "apple"}
				jsonData, err := json.Marshal(&data)
				Expect(err).ToNot(HaveOccurred())

				sentMsg = &container_daemon.RequestMessage{
					Type: container_daemon.ProcessRequest,
					Data: jsonData,
				}
			})

			It("calls the handler with the sent message", func() {
				_, err := connector.Connect(sentMsg)
				Expect(err).ToNot(HaveOccurred())

				Eventually(stubDone).Should(Receive())
				Expect(recvMsg).To(Equal(sentMsg))
			})

			It("gets back the stream the handler provided", func() {
				resp, err := connector.Connect(sentMsg)
				Expect(err).ToNot(HaveOccurred())

				Expect(stubDone).To(Receive())
				Expect(resp.Files).To(HaveLen(2))

				_, err = resp.Files[0].Write([]byte("potato potato"))
				Expect(err).NotTo(HaveOccurred())
				err = resp.Files[0].Close()
				Expect(err).NotTo(HaveOccurred())
				sentFiles[0].Seek(0, 0)
				Expect(ioutil.ReadAll(sentFiles[0])).Should(Equal([]byte("potato potato")))

				_, err = sentFiles[1].Write([]byte("brocoli brocoli"))
				Expect(err).NotTo(HaveOccurred())
				err = sentFiles[1].Close()
				Expect(err).NotTo(HaveOccurred())
				Expect(ioutil.ReadAll(resp.Files[1])).Should(Equal([]byte("brocoli brocoli")))
			})

			It("gets back the pid the handler provided", func() {
				resp, err := connector.Connect(sentMsg)
				Expect(err).ToNot(HaveOccurred())
				Eventually(stubDone).Should(Receive())
				Expect(resp.Pid).To(Equal(sentPid))
			})

			Context("when the handler fails", func() {
				BeforeEach(func() {
					sentErrorMutex.Lock()
					defer sentErrorMutex.Unlock()
					sentError = errors.New("no cake")
				})

				It("sends back the error from the handler", func() {
					_, err := connector.Connect(sentMsg)
					Expect(err).To(MatchError("no cake"))
				})
			})
		})
	})
})
