package container_daemon_test

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_ptyopener"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Spawning", func() {
	var (
		runner    *fake_runner.FakeRunner
		ptyOpener *fake_ptyopener.FakePTYOpener
		spawner   *container_daemon.Spawn

		cmd         *exec.Cmd
		returnedFds []*os.File
		returnedErr error
		withTty     bool
	)

	BeforeEach(func() {
		withTty = false
		runner = new(fake_runner.FakeRunner)
		ptyOpener = new(fake_ptyopener.FakePTYOpener)

		cmd = exec.Command("foo")
		spawner = &container_daemon.Spawn{
			Runner: runner,
			PTY:    ptyOpener,
		}
	})

	JustBeforeEach(func() {
		returnedFds, returnedErr = spawner.Spawn(cmd, withTty)
	})

	Describe("With a TTY", func() {
		BeforeEach(func() {
			withTty = true
		})

		Context("when a pty cannot be opened", func() {
			BeforeEach(func() {
				ptyOpener.OpenReturns(nil, nil, errors.New("an error"))
			})

			It("returns an error", func() {
				Expect(returnedErr).To(HaveOccurred())
			})
		})

		Context("when a pty is opened", func() {
			var theOpenedPTY, theOpenedTTY *os.File

			BeforeEach(func() {
				theOpenedPTY = tmp()
				theOpenedTTY = tmp()
				ptyOpener.OpenReturns(theOpenedPTY, theOpenedTTY, nil)
			})

			It("attaches a tty to the process's stdin, out and error", func() {
				Expect(cmd.Stdin).To(Equal(theOpenedTTY))
				Expect(cmd.Stdout).To(Equal(theOpenedTTY))
				Expect(cmd.Stderr).To(Equal(theOpenedTTY))
			})

			It("returns the pty", func() {
				Expect(returnedFds[0]).To(Equal(theOpenedPTY))
			})

			It("waits for the exit status", func() {
				Eventually(runner.WaitCallCount).Should(Equal(1))
				Expect(runner.WaitArgsForCall(0)).To(Equal(cmd))
			})

			It("tells the command to start with a controlling tty and session id", func() {
				Expect(cmd.SysProcAttr.Setctty).To(Equal(true))
				Expect(cmd.SysProcAttr.Setsid).To(Equal(true))
			})

			Context("when sysprocattr is already set", func() {
				BeforeEach(func() {
					cmd.SysProcAttr = &syscall.SysProcAttr{
						Ptrace: true,
					}
				})

				It("does not clobber it", func() {
					Expect(cmd.SysProcAttr.Ptrace).To(Equal(true))
				})
			})

			Context("after wait returns", func() {
				BeforeEach(func() {
					runner.WaitReturns(42)
				})

				It("sends the exit status returned by wait to the exit status fd", func() {
					Eventually(func() byte {
						exit := make([]byte, 1)
						if n, _ := returnedFds[1].Read(exit); n > 0 {
							return exit[0]
						}

						return 255
					}).Should(Equal(byte(42)))
				})
			})

			Context("when wait does not return", func() {
				var block chan struct{}

				BeforeEach(func() {
					block = make(chan struct{})
					runner.WaitStub = func(*exec.Cmd) byte {
						<-block
						return 0
					}
				})

				It("does not block", func(done Done) {
					close(block)
					Eventually(func() byte {
						exit := make([]byte, 1)
						if n, _ := returnedFds[1].Read(exit); n > 0 {
							return exit[0]
						}

						return 255
					}, 10.0).Should(Equal(byte(0)))
					close(done)
				}, 30.0)
			})
		})
	})

	Describe("Non Interactively (without a tty)", func() {
		var (
			cmdStdin io.Reader
		)

		BeforeEach(func() {
			withTty = false
		})

		Context("when starting the process succeeds", func() {
			BeforeEach(func() {
				cmd = exec.Command("foo")
				runner.StartStub = func(cmd *exec.Cmd) error {
					cmd.Stdout.Write([]byte("Banana doo"))
					cmd.Stderr.Write([]byte("Banana goo"))
					cmdStdin = cmd.Stdin

					return nil
				}
			})

			It("returns the streams from the spawned process", func() {
				Expect(checkReaderContent(returnedFds[1], "Banana doo")).To(BeTrue())
				Expect(checkReaderContent(returnedFds[2], "Banana goo")).To(BeTrue())

				returnedFds[0].Write([]byte("the stdin"))
				returnedFds[0].Close()
				Expect(checkReaderContent(cmdStdin, "the stdin")).To(BeTrue())
			})

			It("waits for the exit status", func() {
				Eventually(runner.WaitCallCount).Should(Equal(1))
				Expect(runner.WaitArgsForCall(0)).To(Equal(cmd))
			})

			Context("after wait returns", func() {
				BeforeEach(func() {
					runner.WaitReturns(42)
				})

				It("sends the exit status returned by wait to the exit status fd", func() {
					Eventually(func() byte {
						exit := make([]byte, 1)
						if n, _ := returnedFds[3].Read(exit); n > 0 {
							return exit[0]
						}

						return 255
					}).Should(Equal(byte(42)))
				})
			})
		})

		Context("when wait does not return", func() {
			var block chan struct{}

			BeforeEach(func() {
				block = make(chan struct{})
				runner.WaitStub = func(*exec.Cmd) byte {
					<-block
					return 0
				}
			})

			It("does not block", func(done Done) {
				close(block)
				Eventually(func() byte {
					exit := make([]byte, 1)
					if n, _ := returnedFds[3].Read(exit); n > 0 {
						return exit[0]
					}

					return 255
				}, 10.0).Should(Equal(byte(0)))
				close(done)
			}, 30.0)
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
