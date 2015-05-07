package container_daemon_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		daemon              container_daemon.ContainerDaemon
		listener            *fake_listener.FakeListener
		runner              *fake_runner.FakeRunner
		pidSettingStartStub func(cmd *exec.Cmd) error
		exitStatusChan      chan byte
		users               *fake_user.FakeUser

		userLookupError error
	)

	const testPid = 3

	etcPasswd := map[string]*user.User{
		"a-user":       &user.User{Uid: "66", Gid: "99"},
		"another-user": &user.User{Uid: "77", Gid: "88", HomeDir: "/the/home/dir"},
		"a-root-user":  &user.User{},
	}

	BeforeEach(func() {
		listener = &fake_listener.FakeListener{}
		runner = new(fake_runner.FakeRunner)
		pidSettingStartStub = func(cmd *exec.Cmd) error {
			cmd.Process = &os.Process{
				Pid: testPid,
			}
			return nil
		}
		users = new(fake_user.FakeUser)
		exitStatusChan = make(chan byte)
		userLookupError = nil

		users.LookupStub = func(name string) (*user.User, error) {
			return etcPasswd[name], userLookupError
		}

		runner.StartStub = pidSettingStartStub

		runner.WaitStub = func(cmd *exec.Cmd) (byte, error) {
			return <-exitStatusChan, nil
		}

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

			var pid int

			BeforeEach(func() {
				spec = &garden.ProcessSpec{
					Path: "fishfinger",
					Args: []string{
						"foo", "bar",
					},
					User: "a-user",
					Dir:  "some-dir",
					Env:  []string{"foo=bar", "baz=barry"},
				}
			})

			JustBeforeEach(func() {
				listener.ListenStub = func(cb unix_socket.ConnectionHandler) error {
					b, err := json.Marshal(spec)
					Expect(err).ToNot(HaveOccurred())

					handleFileHandles, pid, handlerError = cb.Handle(json.NewDecoder(bytes.NewReader(b)))

					return nil
				}

				daemon.Run()
			})

			Context("when command runner succeeds", func() {
				It("spawns a process", func() {
					Expect(runner.StartCallCount()).To(Equal(1))
					exitStatusChan <- 0
				})

				It("returns the PID of the spawned process", func() {
					Expect(pid).To(Equal(testPid))
					exitStatusChan <- 0
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
							exitStatusChan <- 0
						})

						It("has the correct uid", func() {
							Expect(theExecutedCommand.SysProcAttr).ToNot(BeNil())
							Expect(theExecutedCommand.SysProcAttr.Credential).ToNot(BeNil())
							Expect(theExecutedCommand.SysProcAttr.Credential.Uid).To(Equal(uint32(77)))
							Expect(theExecutedCommand.SysProcAttr.Credential.Gid).To(Equal(uint32(88)))
							exitStatusChan <- 0
						})

						It("has the supplied env vars", func() {
							Expect(theExecutedCommand.Env).To(ContainElement("foo=bar"))
							Expect(theExecutedCommand.Env).To(ContainElement("baz=barry"))
							exitStatusChan <- 0
						})

						It("sets the USER environment variable", func() {
							Expect(theExecutedCommand.Env).To(ContainElement("USER=another-user"))
							exitStatusChan <- 0
						})

						It("sets the HOME environment variable to the home dir in /etc/passwd", func() {
							Expect(theExecutedCommand.Env).To(ContainElement("HOME=/the/home/dir"))
							exitStatusChan <- 0
						})

						Context("when the ENV does not contain a PATH", func() {
							Context("and the uid is not 0", func() {
								It("appends the DefaultUserPATH to the environment", func() {
									Expect(theExecutedCommand.Env).To(ContainElement(fmt.Sprintf("PATH=%s", container_daemon.DefaultUserPath)))
									exitStatusChan <- 0
								})
							})

							Context("and the uid is 0", func() {
								BeforeEach(func() {
									spec.User = "a-root-user"
								})

								It("appends the DefaultRootPATH to the environment", func() {
									Expect(theExecutedCommand.Env).To(ContainElement(fmt.Sprintf("PATH=%s", container_daemon.DefaultRootPATH)))
									exitStatusChan <- 0
								})
							})

							Context("when the ENV already contains a PATH", func() {
								BeforeEach(func() {
									spec.Env = []string{"PATH=cake"}
								})

								It("is not overridden", func() {
									Expect(theExecutedCommand.Env).To(ContainElement("PATH=cake"))
									Expect(theExecutedCommand.Env).NotTo(ContainElement(fmt.Sprintf("PATH=%s", container_daemon.DefaultUserPath)))
									exitStatusChan <- 0
								})
							})
						})

						It("has the supplied dir", func() {
							Expect(theExecutedCommand.Dir).To(Equal("some-dir"))
							exitStatusChan <- 0
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

							return pidSettingStartStub(cmd)
						}
					})

					It("returns the streams from the spawned process", func() {
						Expect(checkReaderContent(handleFileHandles[1], "Banana doo")).To(BeTrue())
						Expect(checkReaderContent(handleFileHandles[2], "Banana goo")).To(BeTrue())

						handleFileHandles[0].Write([]byte("the stdin"))
						handleFileHandles[0].Close()
						Expect(checkReaderContent(cmdStdin, "the stdin")).To(BeTrue())

						exitStatusChan <- 0
					})

					It("returns the exit status in an extra stream", func() {
						exitStatusChan <- 43
						b := make([]byte, 1)
						handleFileHandles[3].Read(b)

						Expect(b).To(Equal([]byte{43}))
					})
				})
			})

			Describe("error handling", func() {
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

func checkReaderContent(reader io.Reader, content string) bool {
	buffer := make([]byte, len(content))

	_, err := io.ReadFull(reader, buffer)
	if err != nil {
		panic(err)
	}

	return content == string(buffer)
}
