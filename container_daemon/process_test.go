package container_daemon_test

import (
	"errors"
	"io"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_connector"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Process", func() {
	var socketConnector *fake_connector.FakeConnector

	BeforeEach(func() {
		socketConnector = &fake_connector.FakeConnector{}
	})

	It("sends the correct process payload to the server", func() {
		spec := &garden.ProcessSpec{
			Path: "/bin/echo",
			Args: []string{"Hello world"},
		}

		proc, err := NewProcess(socketConnector, spec, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(proc).ToNot(BeNil())

		Expect(socketConnector.ConnectCallCount()).To(Equal(1))
		Expect(socketConnector.ConnectArgsForCall(0)).To(Equal(spec))
	})

	It("streams stdout back", func() {
		remoteStdout := gbytes.NewBuffer()
		socketConnector.ConnectReturns([]io.ReadWriteCloser{nil, remoteStdout, nil}, nil)

		spec := garden.ProcessSpec{
			Path: "/bin/echo",
			Args: []string{"Hello world"},
		}
		recvStdout := gbytes.NewBuffer()
		io := garden.ProcessIO{
			Stdout: recvStdout,
		}
		_, err := NewProcess(socketConnector, &spec, &io)
		Expect(err).ToNot(HaveOccurred())

		remoteStdout.Write([]byte("Hello world"))
		Eventually(recvStdout).Should(gbytes.Say("Hello world"))
	})

	It("streams stderr back", func() {
		remoteStderr := gbytes.NewBuffer()
		socketConnector.ConnectReturns([]io.ReadWriteCloser{nil, nil, remoteStderr}, nil)

		spec := garden.ProcessSpec{
			Path: "/bin/echo",
			Args: []string{"Hello world"},
		}

		recvStderr := gbytes.NewBuffer()
		io := garden.ProcessIO{
			Stderr: recvStderr,
		}

		_, err := NewProcess(socketConnector, &spec, &io)
		Expect(err).ToNot(HaveOccurred())

		remoteStderr.Write([]byte("Hello world"))
		Eventually(recvStderr).Should(gbytes.Say("Hello world"))
	})

	It("streams stdin over", func() {
		remoteStdin := gbytes.NewBuffer()
		socketConnector.ConnectReturns([]io.ReadWriteCloser{remoteStdin, nil, nil}, nil)

		spec := garden.ProcessSpec{
			Path: "/bin/echo",
			Args: []string{"Hello world"},
		}

		sentStdin := gbytes.NewBuffer()
		io := garden.ProcessIO{
			Stdin: sentStdin,
		}

		_, err := NewProcess(socketConnector, &spec, &io)
		Expect(err).ToNot(HaveOccurred())

		sentStdin.Write([]byte("Hello world"))
		Eventually(remoteStdin).Should(gbytes.Say("Hello world"))
	})

	Context("when it fails to connect", func() {
		It("returns an error", func() {
			socketConnector.ConnectReturns(nil, errors.New("Hoy hoy"))

			spec := garden.ProcessSpec{
				Path: "/bin/echo",
				Args: []string{"Hello world"},
			}

			_, err := NewProcess(socketConnector, &spec, nil)
			Expect(err).To(MatchError("container_daemon: connect to socket: Hoy hoy"))
		})
	})
})
