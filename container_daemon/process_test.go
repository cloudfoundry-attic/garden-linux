package container_daemon_test

import (
	"encoding/json"
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_connector"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_term"
	"github.com/docker/docker/pkg/term"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Process", func() {
	var socketConnector *fake_connector.FakeConnector
	var fakeTerm *fake_term.FakeTerm
	var sigwinchCh chan os.Signal
	var response *container_daemon.ResponseMessage

	var process *container_daemon.Process

	BeforeEach(func() {
		fakeTerm = new(fake_term.FakeTerm)
		socketConnector = new(fake_connector.FakeConnector)
		response = &container_daemon.ResponseMessage{
			Type:  container_daemon.ProcessResponse,
			Files: []container_daemon.StreamingFile{nil, nil, nil, FakeFd(0)},
			Pid:   0,
		}

		socketConnector.ConnectReturns(response, nil)

		sigwinchCh = make(chan os.Signal)

		process = &container_daemon.Process{
			Connector:  socketConnector,
			Term:       fakeTerm,
			SigwinchCh: sigwinchCh,
			Spec: &garden.ProcessSpec{
				Path: "/bin/echo",
				Args: []string{"Hello world"},
			},
			IO: nil,
		}
	})

	It("sends the correct process payload to the server", func() {
		err := process.Start()
		Expect(err).ToNot(HaveOccurred())

		payload, err := json.Marshal(&process.Spec)
		Expect(err).ToNot(HaveOccurred())

		Expect(socketConnector.ConnectCallCount()).To(Equal(1))

		socketMessage := socketConnector.ConnectArgsForCall(0)
		Expect(socketMessage.Data).To(Equal(json.RawMessage(payload)))
	})

	Context("when the process is interactive (i.e. connected to a TTY)", func() {
		BeforeEach(func() {
			process.Spec.TTY = &garden.TTYSpec{}
		})

		It("makes stdin a raw terminal (because the remote terminal will handle echoing etc.)", func() {
			response.Files = []container_daemon.StreamingFile{FakeFd(0), FakeFd(0)}
			response.Pid = 0

			socketConnector.ConnectReturns(response, nil)

			Expect(process.Start()).To(Succeed())
			Expect(fakeTerm.SetRawTerminalCallCount()).To(Equal(1))
		})

		It("restores the terminal state when the process is cleaned up", func() {
			response.Files = []container_daemon.StreamingFile{FakeFd(0), FakeFd(0)}
			response.Pid = 0

			socketConnector.ConnectReturns(response, nil)

			state := &term.State{}
			fakeTerm.SetRawTerminalReturns(state, nil)

			Expect(process.Start()).To(Succeed())
			Expect(fakeTerm.RestoreTerminalCallCount()).To(Equal(0))

			process.Cleanup()
			Expect(fakeTerm.RestoreTerminalCallCount()).To(Equal(1))
			fd, state := fakeTerm.RestoreTerminalArgsForCall(0)
			Expect(fd).To(Equal(os.Stdin.Fd()))
			Expect(state).To(Equal(state))
		})

		It("sets the window size of the process based on the window size of standard input", func() {
			remotePty := FakeFd(123)
			response.Files = []container_daemon.StreamingFile{remotePty, FakeFd(999)}
			response.Pid = 0

			socketConnector.ConnectReturns(response, nil)
			fakeTerm.GetWinsizeReturns(&term.Winsize{
				Width: 1, Height: 2,
			}, nil)

			Expect(process.Start()).To(Succeed())

			Expect(fakeTerm.GetWinsizeCallCount()).To(Equal(1))
			Expect(fakeTerm.GetWinsizeArgsForCall(0)).To(Equal(uintptr(os.Stdin.Fd())))

			Expect(fakeTerm.SetWinsizeCallCount()).To(Equal(1))
			fd, size := fakeTerm.SetWinsizeArgsForCall(0)
			Expect(fd).To(Equal(uintptr(123)))
			Expect(size).To(Equal(&term.Winsize{
				Width: 1, Height: 2,
			}))
		})

		Context("when SIGWINCH is received", func() {
			It("resizes the pty to match the window size of stdin", func() {
				remotePty := FakeFd(123)
				response.Files = []container_daemon.StreamingFile{remotePty, FakeFd(999)}
				response.Pid = 0

				socketConnector.ConnectReturns(response, nil)

				fakeTerm.GetWinsizeReturns(&term.Winsize{
					Width: 3, Height: 4,
				}, nil)

				Expect(process.Start()).To(Succeed())

				Expect(fakeTerm.SetWinsizeCallCount()).To(Equal(1))

				sigwinchCh <- syscall.SIGWINCH

				Eventually(fakeTerm.SetWinsizeCallCount(), 10*time.Second, 500*time.Millisecond).Should(Equal(2))
				fd, size := fakeTerm.SetWinsizeArgsForCall(1)
				Expect(fd).To(Equal(uintptr(123)))
				Expect(size).To(Equal(&term.Winsize{
					Width: 3, Height: 4,
				}))
			})
		})

		It("copies the returned PTYs output to standard output", func() {
			remotePty := FakeFd(0)
			response.Files = []container_daemon.StreamingFile{remotePty, FakeFd(0)}

			socketConnector.ConnectReturns(response, nil)

			recvStdout := FakeFd(0)
			process.IO = &garden.ProcessIO{
				Stdout: recvStdout,
			}

			err := process.Start()
			Expect(err).ToNot(HaveOccurred())

			remotePty.Write([]byte("Hello world"))
			Eventually(recvStdout, "5s").Should(gbytes.Say("Hello world"))
		})

		It("copies standard input to the PTY", func() {
			remotePty := FakeFd(0)
			response.Files = []container_daemon.StreamingFile{remotePty, FakeFd(0)}

			socketConnector.ConnectReturns(response, nil)

			sentStdin := FakeFd(0)
			process.IO = &garden.ProcessIO{
				Stdin: sentStdin,
			}

			err := process.Start()
			Expect(err).ToNot(HaveOccurred())

			sentStdin.Write([]byte("Hello world"))
			Eventually(remotePty, "5s").Should(gbytes.Say("Hello world"))
		})
	})

	It("streams stdout back", func() {
		remoteStdout := FakeFd(0)
		response.Files = []container_daemon.StreamingFile{nil, remoteStdout, nil, FakeFd(0)}
		response.Pid = 0

		socketConnector.ConnectReturns(response, nil)

		recvStdout := FakeFd(0)
		process.IO = &garden.ProcessIO{
			Stdout: recvStdout,
		}

		err := process.Start()
		Expect(err).ToNot(HaveOccurred())

		remoteStdout.Write([]byte("Hello world"))
		Eventually(recvStdout, "5s").Should(gbytes.Say("Hello world"))
	})

	It("streams stderr back", func() {
		remoteStderr := FakeFd(0)
		response.Files = []container_daemon.StreamingFile{nil, nil, remoteStderr, FakeFd(0)}
		response.Pid = 0

		socketConnector.ConnectReturns(response, nil)

		recvStderr := FakeFd(0)
		process.IO = &garden.ProcessIO{
			Stderr: recvStderr,
		}

		err := process.Start()
		Expect(err).ToNot(HaveOccurred())

		remoteStderr.Write([]byte("Hello world"))
		Eventually(recvStderr, "5s").Should(gbytes.Say("Hello world"))
	})

	It("streams stdin over", func() {
		remoteStdin := FakeFd(0)
		response.Files = []container_daemon.StreamingFile{remoteStdin, nil, nil, FakeFd(0)}
		response.Pid = 0

		socketConnector.ConnectReturns(response, nil)

		sentStdin := FakeFd(0)
		process.IO = &garden.ProcessIO{
			Stdin: sentStdin,
		}

		err := process.Start()
		Expect(err).ToNot(HaveOccurred())

		sentStdin.Write([]byte("Hello world"))
		Eventually(remoteStdin, "5s").Should(gbytes.Say("Hello world"))
	})

	It("waits for and reports the correct exit status", func() {
		remoteExitFd := FakeFd(0)
		response.Files = []container_daemon.StreamingFile{nil, nil, nil, remoteExitFd}
		response.Pid = 0

		socketConnector.ConnectReturns(response, nil)

		err := process.Start()
		Expect(err).ToNot(HaveOccurred())

		remoteExitFd.Write([]byte{42})
		Expect(process.Wait()).To(Equal(42))
	})

	Context("when it fails to connect", func() {
		It("returns an error", func() {
			socketConnector.ConnectReturns(nil, errors.New("Hoy hoy"))

			err := process.Start()
			Expect(err).To(MatchError("container_daemon: connect to socket: Hoy hoy"))
		})
	})
})

type fakefd struct {
	fd     uintptr
	buffer *gbytes.Buffer
}

func FakeFd(fd uintptr) *fakefd {
	return &fakefd{fd, gbytes.NewBuffer()}
}

func (f *fakefd) Fd() uintptr {
	return f.fd
}

func (f *fakefd) Buffer() *gbytes.Buffer {
	return f.buffer
}

func (f *fakefd) Read(b []byte) (int, error) {
	return f.buffer.Read(b)
}

func (f *fakefd) Write(b []byte) (int, error) {
	return f.buffer.Write(b)
}

func (f *fakefd) Close() error {
	return f.buffer.Close()
}
