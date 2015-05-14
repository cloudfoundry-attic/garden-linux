package container_daemon_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_listener"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_spawner"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Daemon", func() {
	var (
		daemon   container_daemon.ContainerDaemon
		listener *fake_listener.FakeListener
		spawner  *fake_spawner.FakeSpawner
	)

	BeforeEach(func() {
		listener = &fake_listener.FakeListener{}
		spawner = new(fake_spawner.FakeSpawner)

		daemon = container_daemon.ContainerDaemon{
			Listener: listener,
			Spawner:  spawner,
		}
	})

	Describe("Init", func() {
		It("initializes the listener", func() {
			Expect(daemon.Init()).To(Succeed())
			Expect(listener.InitCallCount()).To(Equal(1))
		})

		Context("when initialization fails", func() {
			It("returns an error", func() {
				listener.InitReturns(errors.New("Hoy hay"))

				err := daemon.Init()
				Expect(err).To(MatchError("container_daemon: initializing the listener: Hoy hay"))
			})
		})
	})

	Describe("Run", func() {
		It("listens for connections", func() {
			Expect(daemon.Run()).To(Succeed())
			Expect(listener.ListenCallCount()).To(Equal(1))
		})

		Context("when a connection is made", func() {
			var handleFileHandles []*os.File
			var handlerError error

			var spec garden.ProcessSpec

			var handlerPid int

			BeforeEach(func() {
				spec = garden.ProcessSpec{
					Path: "fishfinger",
					Args: []string{
						"foo", "bar",
					},
					User: "a-user",
					Dir:  "some-dir",
					Env:  []string{"foo=bar", "baz=barry"},
				}

				spawner.SpawnStub = func(spec garden.ProcessSpec) ([]*os.File, int, error) {
					return nil, 123, nil
				}
			})

			JustBeforeEach(func() {
				listener.ListenStub = func(cb unix_socket.ConnectionHandler) error {
					b, err := json.Marshal(spec)
					Expect(err).ToNot(HaveOccurred())

					handleFileHandles, handlerPid, handlerError = cb.Handle(json.NewDecoder(bytes.NewReader(b)))

					return nil
				}

				daemon.Run()
			})

			It("returns the PID of the spawned process", func() {
				Expect(handlerPid).To(Equal(123))
			})

			Context("if the handler panics", func() {
				BeforeEach(func() {
					spawner.SpawnStub = func(garden.ProcessSpec) ([]*os.File, int, error) {
						panic("bang")
					}
				})

				It("converts the panic to an error", func() {
					Expect(handlerError).To(MatchError("container_daemon: recovered panic: bang"))
				})
			})

			Context("when the spawner returns file handles to the client", func() {
				var someFds []*os.File

				BeforeEach(func() {
					someFds = []*os.File{
						tmp(),
						tmp(),
					}

					spawner.SpawnReturns(someFds, 0, nil)
				})

				AfterEach(func() {
					for _, f := range someFds {
						f.Close()
					}
				})

				It("returns the returned file handles", func() {
					Expect(handleFileHandles).To(Equal(someFds))
				})
			})

			Context("when the spawner returns an error", func() {
				BeforeEach(func() {
					spawner.SpawnReturns(nil, 0, errors.New("will not spawn"))
				})

				It("returns the error to the client", func() {
					Expect(handlerError).To(MatchError("will not spawn"))
				})
			})
		})

		Context("when listening fails", func() {
			It("returns an error", func() {
				listener.ListenReturns(errors.New("Banana foo"))

				err := daemon.Run()
				Expect(err).To(MatchError("container_daemon: listening for connections: Banana foo"))
			})
		})
	})

	Describe("Stop", func() {
		It("stops the listener", func() {
			daemon.Run()
			Expect(daemon.Stop()).To(Succeed())
			Expect(listener.StopCallCount()).To(Equal(1))
		})

		Context("when it failes to stop the listener", func() {
			It("returns an error", func() {
				listener.StopReturns(errors.New("Ping pong"))

				err := daemon.Stop()
				Expect(err).To(MatchError("container_daemon: stopping the listener: Ping pong"))
			})
		})
	})
})

func tmp() *os.File {
	f, err := ioutil.TempFile("", "")
	Expect(err).NotTo(HaveOccurred())
	return f
}
