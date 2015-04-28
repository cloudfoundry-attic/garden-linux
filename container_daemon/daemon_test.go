package container_daemon_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_listener"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_runner"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system/fake_user"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Daemon", func() {
	var (
		daemon   container_daemon.ContainerDaemon
		listener *fake_listener.FakeListener
		runner   *fake_runner.FakeRunner
		users    *fake_user.FakeUser

		userLookupError error
	)

	etcPasswd := map[string]*user.User{
		"a-user":       &user.User{Uid: "66", Gid: "99"},
		"another-user": &user.User{Uid: "77", Gid: "88"},
	}

	BeforeEach(func() {
		listener = &fake_listener.FakeListener{}
		runner = new(fake_runner.FakeRunner)
		users = new(fake_user.FakeUser)
		userLookupError = nil

		users.LookupStub = func(name string) (*user.User, error) {
			return etcPasswd[name], userLookupError
		}

		runner.WaitReturns(43, nil)

		daemon = container_daemon.ContainerDaemon{
			Listener: listener,
			Users:    users,
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

			var spec *garden.ProcessSpec

			BeforeEach(func() {
				spec = &garden.ProcessSpec{
					Path: "fishfinger",
					Args: []string{
						"foo", "bar",
					},
					User: "a-user",
				}
			})

			JustBeforeEach(func() {
				listener.ListenStub = func(cb unix_socket.ConnectionHandler) error {
					b, err := json.Marshal(spec)
					Expect(err).ToNot(HaveOccurred())

					handleFileHandles, handlerError = cb.Handle(json.NewDecoder(bytes.NewReader(b)))

					return nil
				}

				daemon.Run()
			})

			It("spawns a process", func() {
				Expect(runner.StartCallCount()).To(Equal(1))
			})

			Describe("the spawned process", func() {
				Context("when the process spec names a user which exists in /etc/passwd", func() {
					var theExecutedCommand *exec.Cmd

					JustBeforeEach(func() {
						Expect(runner.StartCallCount()).To(Equal(1))
						theExecutedCommand = runner.StartArgsForCall(0)
					})

					BeforeEach(func() {
						spec.User = "another-user"
					})

					It("has the correct path and args", func() {
						Expect(theExecutedCommand.Path).To(Equal("fishfinger"))
						Expect(theExecutedCommand.Args).To(Equal([]string{"fishfinger", "foo", "bar"}))
					})

					It("has the correct uid", func() {
						Expect(theExecutedCommand.SysProcAttr).ToNot(BeNil())
						Expect(theExecutedCommand.SysProcAttr.Credential).ToNot(BeNil())
						Expect(theExecutedCommand.SysProcAttr.Credential.Uid).To(Equal(uint32(77)))
						Expect(theExecutedCommand.SysProcAttr.Credential.Gid).To(Equal(uint32(88)))
					})
				})

				Context("when the process spec names a user which exists in /etc/passwd", func() {
					BeforeEach(func() {
						spec.User = "not-a-user"
					})

					It("returns an informative error", func() {
						Expect(handlerError).To(MatchError("container_daemon: failed to lookup user not-a-user"))
					})
				})

				Context("when the process spec names a user which exists in /etc/passwd", func() {
					BeforeEach(func() {
						spec.User = "not-a-user"
						userLookupError = errors.New("boom")
					})

					It("returns an informative error", func() {
						Expect(handlerError).To(MatchError("container_daemon: lookup user not-a-user: boom"))
					})
				})
			})

			Context("when the process returns output", func() {
				var cmdStdin io.Reader

				BeforeEach(func() {
					runner.StartStub = func(cmd *exec.Cmd) error {
						cmd.Stdout.Write([]byte("Banana doo"))
						cmd.Stderr.Write([]byte("Banana goo"))
						cmdStdin = cmd.Stdin

						return nil
					}
				})

				It("returns the streams from the spawned process", func() {
					Expect(ioutil.ReadAll(handleFileHandles[1])).To(Equal([]byte("Banana doo")))
					Expect(ioutil.ReadAll(handleFileHandles[2])).To(Equal([]byte("Banana goo")))

					handleFileHandles[0].Write([]byte("the stdin"))
					handleFileHandles[0].Close()
					Expect(ioutil.ReadAll(cmdStdin)).To(Equal([]byte("the stdin")))
				})

				It("returns the exit status in an extra stream", func() {
					b := make([]byte, 1)
					handleFileHandles[3].Read(b)

					Expect(b).To(Equal([]byte{43}))
				})
			})

			Context("when command runner fails", func() {
				BeforeEach(func() {
					runner.StartStub = func(cmd *exec.Cmd) error {
						return errors.New("Banana blue")
					}
				})

				It("returns an error", func() {
					Expect(handlerError).To(MatchError("container_daemon: running command: Banana blue"))
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
