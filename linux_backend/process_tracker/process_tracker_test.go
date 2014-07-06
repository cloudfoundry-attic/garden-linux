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

	It("runs the command asynchronously via iomux-spawn", func() {
		cmd := &exec.Cmd{Path: "/bin/bash"}

		cmd.Stdin = bytes.NewBufferString("echo hi")

		setupSuccessfulSpawn()

		process, err := processTracker.Run(cmd, warden.ProcessIO{})
		Expect(err).NotTo(HaveOccurred())

		Eventually(fakeRunner).Should(HaveBackgrounded(
			fake_command_runner.CommandSpec{
				Path: "bash",
				Args: []string{
					"-c",
					"cat | " + binPath("iomux-spawn") + ` "$@" &`,
					binPath("iomux-spawn"),
					fmt.Sprintf(tmpdir+"/depot/some-id/processes/%d", process.ID()),
					"/bin/bash",
				},
				Stdin: "echo hi",
			},
		))
	})

	It("initiates a link to the process after spawn is ready", func(done Done) {
		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: "bash",
			}, func(cmd *exec.Cmd) error {
				go func() {
					defer GinkgoRecover()

					time.Sleep(100 * time.Millisecond)

					Expect(fakeRunner).ToNot(HaveStartedExecuting(
						fake_command_runner.CommandSpec{
							Path: binPath("iomux-link"),
						},
					), "Executed iomux-link too early!")

					Ω(cmd.Stdout).ShouldNot(BeNil())

					fakeRunner.WhenWaitingFor(
						fake_command_runner.CommandSpec{
							Path: binPath("iomux-link"),
						},
						func(*exec.Cmd) error {
							close(done)
							return nil
						},
					)

					cmd.Stdout.Write([]byte("xxx\n"))

					Eventually(fakeRunner).Should(HaveStartedExecuting(
						fake_command_runner.CommandSpec{
							Path: binPath("iomux-link"),
						},
					))
				}()

				return nil
			},
		)

		processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
	}, 10.0)

	It("returns unique process IDs", func() {
		setupSuccessfulSpawn()

		process1, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
		Expect(err).NotTo(HaveOccurred())

		process2, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
		Expect(err).NotTo(HaveOccurred())

		Ω(process1.ID()).ShouldNot(Equal(process2.ID()))
	})

	It("creates the process's working directory", func() {
		setupSuccessfulSpawn()

		process, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
		Expect(err).NotTo(HaveOccurred())

		_, err = os.Stat(fmt.Sprintf(tmpdir+"/depot/some-id/processes/%d", process.ID()))
		Expect(err).NotTo(HaveOccurred())
	})

	It("streams output from the process", func() {
		setupSuccessfulSpawn()

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iomux-link"),
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
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(stdout).Should(gbytes.Say("hi out\n"))
		Eventually(stderr).Should(gbytes.Say("hi err\n"))
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
			_, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
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

		processTracker.Restore(0)

		cmd := &exec.Cmd{Path: "/bin/bash"}

		cmd.Stdin = bytes.NewBufferString("echo hi")

		process, err := processTracker.Run(cmd, warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.ID()).Should(Equal(uint32(1)))

		processTracker.Restore(5)

		cmd = &exec.Cmd{Path: "/bin/bash"}

		cmd.Stdin = bytes.NewBufferString("echo hi")

		process, err = processTracker.Run(cmd, warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.ID()).Should(Equal(uint32(6)))
	})

	It("tracks the restored process", func() {
		processTracker.Restore(2)

		activeProcesses := processTracker.ActiveProcessIDs()

		Ω(activeProcesses).Should(Equal([]uint32{2}))
	})
})

var _ = Describe("Attaching to running processes", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New(tmpdir+"/depot/some-id", fakeRunner)

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iomux-link"),
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
	})

	It("streams their stdout and stderr into the channel", func() {
		setupSuccessfulSpawn()

		stdout := gbytes.NewBuffer()
		stderr := gbytes.NewBuffer()

		process, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
		Expect(err).NotTo(HaveOccurred())

		process, err = processTracker.Attach(process.ID(), warden.ProcessIO{
			Stdout: stdout,
			Stderr: stderr,
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(stdout).Should(gbytes.Say("hi out\n"))
		Eventually(stderr).Should(gbytes.Say("hi err\n"))
	})

	Context("when the process is not yet linked to", func() {
		It("runs iomux-link", func() {
			setupSuccessfulSpawn()

			processTracker.Restore(0)

			Ω(fakeRunner).ShouldNot(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: binPath("iomux-link"),
				},
			))

			_, err := processTracker.Attach(0, warden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(fakeRunner).Should(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: binPath("iomux-link"),
				},
			))
		})
	})

	Context("when the process completes", func() {
		It("yields the exit status and closes the channel", func() {
			setupSuccessfulSpawn()

			process, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
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

	It("sends SIGINT to in-flight iomux-links", func() {
		setupSuccessfulSpawn()

		linked := make(chan bool, 2)

		fakeRunner.WhenWaitingFor(
			fake_command_runner.CommandSpec{
				Path: binPath("iomux-link"),
			},
			func(cmd *exec.Cmd) error {
				linked <- true
				select {}
				return nil
			},
		)

		_, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		_, err = processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(linked).Should(Receive())
		Eventually(linked).Should(Receive())

		processTracker.UnlinkAll()

		Ω(fakeRunner).Should(HaveSignalled(
			fake_command_runner.CommandSpec{
				Path: tmpdir + "/depot/some-id/bin/iomux-link",
				Args: []string{
					"-w", tmpdir + "/depot/some-id/processes/0/cursors",
					tmpdir + "/depot/some-id/processes/0",
				},
			},
			os.Interrupt,
		))

		Ω(fakeRunner).Should(HaveSignalled(
			fake_command_runner.CommandSpec{
				Path: tmpdir + "/depot/some-id/bin/iomux-link",
				Args: []string{
					"-w", tmpdir + "/depot/some-id/processes/1/cursors",
					tmpdir + "/depot/some-id/processes/1",
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

		running := make(chan []uint32, 2)

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iomux-link"),
			},
			func(cmd *exec.Cmd) error {
				running <- processTracker.ActiveProcessIDs()
				return nil
			},
		)

		process1, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		process2, err := processTracker.Run(exec.Command("xxx"), warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		runningIDs := append(<-running, <-running...)

		Ω(runningIDs).Should(ContainElement(process1.ID()))
		Ω(runningIDs).Should(ContainElement(process2.ID()))
	})
})
