package container_daemon_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_cmdpreparer"
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
		preparer *fake_cmdpreparer.FakeCmdPreparer
	)

	BeforeEach(func() {
		listener = &fake_listener.FakeListener{}
		spawner = new(fake_spawner.FakeSpawner)
		preparer = new(fake_cmdpreparer.FakeCmdPreparer)

		daemon = container_daemon.ContainerDaemon{
			CmdPreparer: preparer,
			Spawner:     spawner,
		}
	})

	Describe("Run", func() {
		It("listens for connections", func() {
			Expect(daemon.Run(listener)).To(Succeed())
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

				preparer.PrepareCmdReturns(exec.Command("foo"), nil)

				spawner.SpawnStub = func(cmd *exec.Cmd, withTty bool) ([]*os.File, error) {
					cmd.Process = &os.Process{Pid: 123}
					return nil, nil
				}
			})

			JustBeforeEach(func() {
				listener.ListenStub = func(cb unix_socket.ConnectionHandler) error {
					b, err := json.Marshal(spec)
					Expect(err).ToNot(HaveOccurred())

					handleFileHandles, handlerPid, handlerError = cb.Handle(json.NewDecoder(bytes.NewReader(b)))

					return nil
				}

				daemon.Run(listener)
			})

			It("runs the spawner with a prepared command", func() {
				Expect(preparer.PrepareCmdCallCount()).To(Equal(1))
				Expect(preparer.PrepareCmdArgsForCall(0)).To(Equal(spec))
				Expect(spawner.SpawnCallCount()).To(Equal(1))
			})

			It("returns the PID of the spawned process", func() {
				Expect(handlerPid).To(Equal(123))
			})

			Context("when a null TTYSpec is passed", func() {
				It("asks to spawn with a tty", func() {
					_, spawnWithTty := spawner.SpawnArgsForCall(0)
					Expect(spawnWithTty).To(Equal(false))
				})
			})

			Context("when a non-null TTYSpec is passed", func() {
				BeforeEach(func() {
					spec.TTY = &garden.TTYSpec{}
				})

				It("asks to spawn with a tty", func() {
					_, spawnWithTty := spawner.SpawnArgsForCall(0)
					Expect(spawnWithTty).To(Equal(true))
				})
			})

			Context("when the preparer returns an error", func() {
				BeforeEach(func() {
					preparer.PrepareCmdReturns(nil, errors.New("no cmd"))
				})

				It("does not run the spawner", func() {
					Expect(spawner.SpawnCallCount()).To(Equal(0))
				})

				It("returns an error", func() {
					Expect(handlerError).To(HaveOccurred())
				})
			})

			Context("if the handler panics", func() {
				BeforeEach(func() {
					preparer.PrepareCmdStub = func(garden.ProcessSpec) (*exec.Cmd, error) {
						panic("boom")
					}

					spawner.SpawnStub = func(*exec.Cmd, bool) ([]*os.File, error) {
						panic("bang")
					}
				})

				It("converts the panic to an error", func() {
					Expect(handlerError).To(MatchError("container_daemon: recovered panic: boom"))
				})
			})

			Context("when the spawner returns file handles to the client", func() {
				var someFds []*os.File

				BeforeEach(func() {
					someFds = []*os.File{
						tmp(),
						tmp(),
					}

					spawner.SpawnReturns(someFds, nil)
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
					spawner.SpawnReturns(nil, errors.New("will not spawn"))
				})

				It("returns the error to the client", func() {
					Expect(handlerError).To(MatchError("will not spawn"))
				})
			})
		})

		Context("when listening fails", func() {
			It("returns an error", func() {
				listener.ListenReturns(errors.New("Banana foo"))

				err := daemon.Run(listener)
				Expect(err).To(MatchError("container_daemon: listening for connections: Banana foo"))
			})
		})
	})
})

func tmp() *os.File {
	f, err := ioutil.TempFile("", "")
	Expect(err).NotTo(HaveOccurred())
	return f
}
