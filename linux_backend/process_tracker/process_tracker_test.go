package process_tracker_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/process_tracker"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var fakeRunner *fake_command_runner.FakeCommandRunner
var processTracker process_tracker.ProcessTracker

var tmpdir string

var _ = BeforeEach(func() {
	var err error

	tmpdir, err = ioutil.TempDir("", "process-tracker-tests")
	Ω(err).ShouldNot(HaveOccurred())
})

var _ = AfterEach(func() {
	os.RemoveAll(tmpdir)
})

func binPath(bin string) string {
	return path.Join(tmpdir+"/depot/some-id", "bin", bin)
}

func setupSuccessfulSpawn() {
	fakeRunner.WhenRunning(
		fake_command_runner.CommandSpec{
			Path: "bash",
		},
		func(cmd *exec.Cmd) error {
			cmd.Stdout.Write([]byte("ready\n"))
			cmd.Stdout.Write([]byte("active\n"))
			return nil
		},
	)
}

var _ = Describe("Running processes", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New(tmpdir+"/depot/some-id", fakeRunner)
	})

	It("runs the command asynchronously via iodaemon spawn", func() {
		cmd := &exec.Cmd{Path: "/bin/bash", Args: []string{"-l"}}

		setupSuccessfulSpawn()

		process, err := processTracker.Run(cmd, warden.ProcessIO{}, false)
		Expect(err).NotTo(HaveOccurred())

		Eventually(fakeRunner).Should(HaveBackgrounded(
			fake_command_runner.CommandSpec{
				Path: "bash",
				Args: []string{
					"-c",
					binPath("iodaemon") + ` "$@" &`,
					binPath("iodaemon"),
					"-tty=false",
					"spawn",
					fmt.Sprintf(tmpdir+"/depot/some-id/processes/%d.sock", process.ID()),
					"/bin/bash",
					"-l",
				},
			},
		))
	})

	It("initiates a link to the process after spawn is ready", func(done Done) {
		linkSpec := fake_command_runner.CommandSpec{
			Path: binPath("iodaemon"),
			Args: []string{
				"-tty=false",
				"link",
				tmpdir + "/depot/some-id/processes/1.sock",
			},
		}

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: "bash",
			}, func(cmd *exec.Cmd) error {
				go func() {
					defer GinkgoRecover()

					time.Sleep(100 * time.Millisecond)

					Expect(fakeRunner).ToNot(HaveStartedExecuting(
						linkSpec,
					), "Executed iodaemon too early!")

					Ω(cmd.Stdout).ShouldNot(BeNil())

					fakeRunner.WhenWaitingFor(linkSpec, func(*exec.Cmd) error {
						close(done)
						return nil
					})

					cmd.Stdout.Write([]byte("xxx\n"))

					Eventually(fakeRunner).Should(HaveStartedExecuting(linkSpec))
				}()

				return nil
			},
		)

		processTracker.Run(exec.Command("xxx"), warden.ProcessIO{}, false)
	}, 10.0)

	It("returns unique process IDs", func() {
		setupSuccessfulSpawn()

		process1, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{}, false)
		Expect(err).NotTo(HaveOccurred())

		process2, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{}, false)
		Expect(err).NotTo(HaveOccurred())

		Ω(process1.ID()).ShouldNot(Equal(process2.ID()))
	})

	It("streams output from the process", func() {
		setupSuccessfulSpawn()

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iodaemon"),
				Args: []string{
					"-tty=false",
					"link",
					tmpdir + "/depot/some-id/processes/1.sock",
				},
			},
			func(cmd *exec.Cmd) error {
				cmd.Stdout.Write([]byte("hi out\n"))
				cmd.Stderr.Write([]byte("hi err\n"))

				dummyCmd := exec.Command("/bin/bash", "-c", "exit 42")
				dummyCmd.Run()

				cmd.ProcessState = dummyCmd.ProcessState

				return nil
			},
		)

		stdout := gbytes.NewBuffer()
		stderr := gbytes.NewBuffer()

		_, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{
			Stdout: stdout,
			Stderr: stderr,
		}, false)
		Expect(err).NotTo(HaveOccurred())

		Eventually(stdout).Should(gbytes.Say("hi out\n"))
		Eventually(stderr).Should(gbytes.Say("hi err\n"))
	})

	It("streams input to the process", func() {
		setupSuccessfulSpawn()

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iodaemon"),
				Args: []string{
					"-tty=false",
					"link",
					tmpdir + "/depot/some-id/processes/1.sock",
				},
			},
			func(cmd *exec.Cmd) error {
				stdin, err := ioutil.ReadAll(cmd.Stdin)
				Ω(err).ShouldNot(HaveOccurred())

				fmt.Fprintf(cmd.Stdout, "roundtripped %s\n", stdin)

				dummyCmd := exec.Command("/bin/bash", "-c", "exit 42")
				dummyCmd.Run()

				cmd.ProcessState = dummyCmd.ProcessState

				return nil
			},
		)

		stdout := gbytes.NewBuffer()

		_, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{
			Stdin:  bytes.NewBufferString("hi in"),
			Stdout: stdout,
		}, false)
		Expect(err).NotTo(HaveOccurred())

		Eventually(stdout).Should(gbytes.Say("roundtripped hi in\n"))
	})

	Context("with a tty", func() {
		It("spawns and links with -tty", func() {
			cmd := &exec.Cmd{Path: "/bin/bash", Args: []string{"-l"}}

			setupSuccessfulSpawn()

			process, err := processTracker.Run(cmd, warden.ProcessIO{}, true)
			Expect(err).NotTo(HaveOccurred())

			Eventually(fakeRunner).Should(HaveBackgrounded(
				fake_command_runner.CommandSpec{
					Path: "bash",
					Args: []string{
						"-c",
						binPath("iodaemon") + ` "$@" &`,
						binPath("iodaemon"),
						"-tty=true",
						"spawn",
						fmt.Sprintf(tmpdir+"/depot/some-id/processes/%d.sock", process.ID()),
						"/bin/bash",
						"-l",
					},
				},
			))

			Eventually(fakeRunner).Should(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: binPath("iodaemon"),
					Args: []string{
						"-tty=true",
						"link",
						fmt.Sprintf(tmpdir+"/depot/some-id/processes/%d.sock", process.ID()),
					},
				},
			))
		})
	})

	Context("when spawning fails", func() {
		disaster := errors.New("oh no!")

		BeforeEach(func() {
			fakeRunner.WhenRunning(
				fake_command_runner.CommandSpec{
					Path: "bash",
				}, func(*exec.Cmd) error {
					return disaster
				},
			)
		})

		It("returns the error", func() {
			_, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{}, false)
			Ω(err).Should(Equal(disaster))
		})
	})
})

var _ = Describe("Restoring processes", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New(tmpdir+"/depot/some-id", fakeRunner)
	})

	It("makes the next process ID be higher than the highest restored ID", func() {
		setupSuccessfulSpawn()

		processTracker.Restore(0, false)

		cmd := &exec.Cmd{Path: "/bin/bash"}

		cmd.Stdin = bytes.NewBufferString("echo hi")

		process, err := processTracker.Run(cmd, warden.ProcessIO{}, false)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.ID()).Should(Equal(uint32(1)))

		processTracker.Restore(5, false)

		cmd = &exec.Cmd{Path: "/bin/bash"}

		cmd.Stdin = bytes.NewBufferString("echo hi")

		process, err = processTracker.Run(cmd, warden.ProcessIO{}, false)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.ID()).Should(Equal(uint32(6)))
	})

	It("tracks the restored process", func() {
		processTracker.Restore(2, false)

		activeProcesses := processTracker.ActiveProcesses()
		Ω(activeProcesses).Should(HaveLen(1))
		Ω(activeProcesses[0].ID()).Should(Equal(uint32(2)))
	})
})

var _ = Describe("Attaching to running processes", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New(tmpdir+"/depot/some-id", fakeRunner)

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iodaemon"),
				Args: []string{
					"-tty=false",
					"link",
					tmpdir + "/depot/some-id/processes/1.sock",
				},
			},
			func(cmd *exec.Cmd) error {
				stdin, err := ioutil.ReadAll(cmd.Stdin)
				Ω(err).ShouldNot(HaveOccurred())

				fmt.Fprintf(cmd.Stdout, "roundtripped %s\n", stdin)

				cmd.Stdout.Write([]byte("hi out\n"))
				cmd.Stderr.Write([]byte("hi err\n"))

				dummyCmd := exec.Command("/bin/bash", "-c", "exit 42")
				dummyCmd.Run()

				cmd.ProcessState = dummyCmd.ProcessState

				return nil
			},
		)
	})

	It("streams stdout, stdin, and stderr", func() {
		setupSuccessfulSpawn()

		stdout := gbytes.NewBuffer()
		stderr := gbytes.NewBuffer()

		process, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{}, false)
		Expect(err).NotTo(HaveOccurred())

		process, err = processTracker.Attach(process.ID(), warden.ProcessIO{
			Stdin:  bytes.NewBufferString("hi in"),
			Stdout: stdout,
			Stderr: stderr,
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(stdout).Should(gbytes.Say("roundtripped hi in\n"))
		Eventually(stdout).Should(gbytes.Say("hi out\n"))
		Eventually(stderr).Should(gbytes.Say("hi err\n"))
	})

	Context("when the process is not yet linked to", func() {
		It("runs iodaemon link", func() {
			setupSuccessfulSpawn()

			processTracker.Restore(1, false)

			Ω(fakeRunner).ShouldNot(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: binPath("iodaemon"),
					Args: []string{
						"-tty=false",
						"link",
						tmpdir + "/depot/some-id/processes/1.sock",
					},
				},
			))

			_, err := processTracker.Attach(1, warden.ProcessIO{
				Stdin: bytes.NewBufferString("hi in"),
			})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(fakeRunner).Should(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: binPath("iodaemon"),
					Args: []string{
						"-tty=false",
						"link",
						tmpdir + "/depot/some-id/processes/1.sock",
					},
				},
			))
		})
	})

	Context("when the process completes", func() {
		It("yields the exit status and closes the channel", func() {
			setupSuccessfulSpawn()

			process, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{
				Stdin: bytes.NewBufferString("hi in"),
			}, false)
			Expect(err).NotTo(HaveOccurred())

			Ω(process.Wait()).Should(Equal(42))
		})
	})
})

var _ = Describe("Unlinking active processes", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New(tmpdir+"/depot/some-id", fakeRunner)
	})

	It("sends SIGINT to in-flight iodaemon links", func() {
		setupSuccessfulSpawn()

		linked := make(chan bool, 2)

		fakeRunner.WhenWaitingFor(
			fake_command_runner.CommandSpec{
				Path: binPath("iodaemon"),
			},
			func(cmd *exec.Cmd) error {
				linked <- true
				select {}
				return nil
			},
		)

		_, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{}, false)
		Ω(err).ShouldNot(HaveOccurred())

		_, err = processTracker.Run(exec.Command("xxx"), warden.ProcessIO{}, false)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(linked).Should(Receive())
		Eventually(linked).Should(Receive())

		processTracker.UnlinkAll()

		Ω(fakeRunner).Should(HaveSignalled(
			fake_command_runner.CommandSpec{
				Path: tmpdir + "/depot/some-id/bin/iodaemon",
				Args: []string{
					"-tty=false",
					"link",
					tmpdir + "/depot/some-id/processes/1.sock",
				},
			},
			os.Interrupt,
		))

		Ω(fakeRunner).Should(HaveSignalled(
			fake_command_runner.CommandSpec{
				Path: tmpdir + "/depot/some-id/bin/iodaemon",
				Args: []string{
					"-tty=false",
					"link",
					tmpdir + "/depot/some-id/processes/2.sock",
				},
			},
			os.Interrupt,
		))
	})
})

var _ = Describe("Listing active process IDs", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New(tmpdir+"/depot/some-id", fakeRunner)
	})

	It("includes running process IDs", func() {
		setupSuccessfulSpawn()

		running := make(chan []process_tracker.LinuxProcess, 2)

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iodaemon"),
				Args: []string{
					"-tty=false",
					"link",
					tmpdir + "/depot/some-id/processes/1.sock",
				},
			},
			func(cmd *exec.Cmd) error {
				running <- processTracker.ActiveProcesses()
				return nil
			},
		)

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iodaemon"),
				Args: []string{
					"-tty=false",
					"link",
					tmpdir + "/depot/some-id/processes/2.sock",
				},
			},
			func(cmd *exec.Cmd) error {
				running <- processTracker.ActiveProcesses()
				return nil
			},
		)

		process1, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{}, false)
		Ω(err).ShouldNot(HaveOccurred())

		process2, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{}, false)
		Ω(err).ShouldNot(HaveOccurred())

		runningIDs := append(<-running, <-running...)

		Ω(runningIDs).Should(ContainElement(process1))
		Ω(runningIDs).Should(ContainElement(process2))
	})
})
