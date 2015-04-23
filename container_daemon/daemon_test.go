package container_daemon_test

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_listener"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Daemon", func() {
	var (
		daemon   container_daemon.ContainerDaemon
		listener *fake_listener.FakeListener
		runner   *fake_command_runner.FakeCommandRunner
	)

	BeforeEach(func() {
		listener = &fake_listener.FakeListener{}
		runner = fake_command_runner.New()

		daemon = container_daemon.ContainerDaemon{
			Listener: listener,
			Runner:   runner,
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

			JustBeforeEach(func() {
				listener.ListenStub = func(cb unix_socket.ConnectionHandler) error {
					decoder := json.NewDecoder(strings.NewReader("{\"path\": \"fishfinger\", \"args\": [\"foo\", \"bar\"]}"))
					handleFileHandles, handlerError = cb.Handle(decoder)

					return nil
				}

				daemon.Run()
			})

			It("spawns a process", func() {
				Expect(runner).To(HaveStartedExecuting(fake_command_runner.CommandSpec{
					Path: "fishfinger",
					Args: []string{"foo", "bar"},
				}))
			})

			Context("when the process returns output", func() {
				var cmdStdin io.Reader

				BeforeEach(func() {
					runner.WhenRunning(fake_command_runner.CommandSpec{}, func(cmd *exec.Cmd) error {
						cmd.Stdout.Write([]byte("Banana doo"))
						cmd.Stderr.Write([]byte("Banana goo"))
						cmdStdin = cmd.Stdin
						return nil
					})
				})

				It("returns the streams from the spawned process", func() {
					Expect(ioutil.ReadAll(handleFileHandles[1])).To(Equal([]byte("Banana doo")))
					Expect(ioutil.ReadAll(handleFileHandles[2])).To(Equal([]byte("Banana goo")))

					handleFileHandles[0].Write([]byte("the stdin"))
					handleFileHandles[0].Close()
					Expect(ioutil.ReadAll(cmdStdin)).To(Equal([]byte("the stdin")))
				})
			})

			Context("when command runner fails", func() {
				BeforeEach(func() {
					runner.WhenRunning(fake_command_runner.CommandSpec{}, func(cmd *exec.Cmd) error {
						return errors.New("Banana blue")
					})
				})

				It("returns an error", func() {
					Expect(handlerError).To(MatchError("running command: Banana blue"))
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
				Expect(err).To(MatchError("container_daemon: stoping the listener: Ping pong"))
			})
		})
	})
})
