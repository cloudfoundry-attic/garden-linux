package process_tracker_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/process_tracker"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var fakeRunner *fake_command_runner.FakeCommandRunner
var processTracker process_tracker.ProcessTracker

func binPath(bin string) string {
	return path.Join("/depot/some-id", "bin", bin)
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
		processTracker = process_tracker.New("/depot/some-id", fakeRunner)
	})

	It("runs the command asynchronously via iomux-spawn", func() {
		cmd := &exec.Cmd{Path: "/bin/bash"}

		cmd.Stdin = bytes.NewBufferString("echo hi")

		setupSuccessfulSpawn()

		processID, _, err := processTracker.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		Eventually(fakeRunner).Should(HaveBackgrounded(
			fake_command_runner.CommandSpec{
				Path: "bash",
				Args: []string{
					"-c",
					"cat | " + binPath("iomux-spawn") + ` "$@" &`,
					binPath("iomux-spawn"),
					fmt.Sprintf("/depot/some-id/processes/%d", processID),
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

					Expect(cmd.Stdout).ToNot(BeNil())

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

		processTracker.Run(exec.Command("xxx"))
	}, 10.0)

	It("returns a unique process ID", func() {
		setupSuccessfulSpawn()

		processID1, _, err := processTracker.Run(exec.Command("xxx"))
		Expect(err).NotTo(HaveOccurred())
		processID2, _, err := processTracker.Run(exec.Command("xxx"))
		Expect(err).NotTo(HaveOccurred())

		Expect(processID1).ToNot(Equal(processID2))
	})

	It("creates the process's working directory", func() {
		setupSuccessfulSpawn()

		processID, _, err := processTracker.Run(exec.Command("xxx"))
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRunner).To(HaveExecutedSerially(
			fake_command_runner.CommandSpec{
				Path: "mkdir",
				Args: []string{
					"-p",
					fmt.Sprintf("/depot/some-id/processes/%d", processID),
				},
			},
		))
	})

	It("streams output from the process", func(done Done) {
		setupSuccessfulSpawn()

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iomux-link"),
			},
			func(cmd *exec.Cmd) error {
				time.Sleep(100 * time.Millisecond)

				cmd.Stdout.Write([]byte("hi out\n"))

				time.Sleep(100 * time.Millisecond)

				cmd.Stderr.Write([]byte("hi err\n"))

				time.Sleep(100 * time.Millisecond)

				dummyCmd := exec.Command("/bin/bash", "-c", "exit 42")
				dummyCmd.Run()

				cmd.ProcessState = dummyCmd.ProcessState

				return nil
			},
		)

		_, processStreamChannel, err := processTracker.Run(exec.Command("xxx"))
		Expect(err).NotTo(HaveOccurred())

		chunk1 := <-processStreamChannel
		Expect(chunk1.Source).To(Equal(warden.ProcessStreamSourceStdout))
		Expect(string(chunk1.Data)).To(Equal("hi out\n"))
		Expect(chunk1.ExitStatus).To(BeNil())

		chunk2 := <-processStreamChannel
		Expect(chunk2.Source).To(Equal(warden.ProcessStreamSourceStderr))
		Expect(string(chunk2.Data)).To(Equal("hi err\n"))
		Expect(chunk2.ExitStatus).To(BeNil())

		close(done)
	}, 5.0)

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
			_, _, err := processTracker.Run(exec.Command("xxx"))
			Expect(err).To(Equal(disaster))
		})
	})
})

var _ = Describe("Restoring processes", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New("/depot/some-id", fakeRunner)
	})

	It("makes the next process ID be higher than the highest restored ID", func() {
		setupSuccessfulSpawn()

		processTracker.Restore(0)

		cmd := &exec.Cmd{Path: "/bin/bash"}

		cmd.Stdin = bytes.NewBufferString("echo hi")

		processID, _, err := processTracker.Run(cmd)
		Expect(err).ToNot(HaveOccurred())
		Expect(processID).To(Equal(uint32(1)))

		processTracker.Restore(5)

		cmd = &exec.Cmd{Path: "/bin/bash"}

		cmd.Stdin = bytes.NewBufferString("echo hi")

		processID, _, err = processTracker.Run(cmd)
		Expect(err).ToNot(HaveOccurred())
		Expect(processID).To(Equal(uint32(6)))
	})

	It("tracks the restored process", func() {
		processTracker.Restore(2)

		activeProcesses := processTracker.ActiveProcessIDs()

		Expect(activeProcesses).To(Equal([]uint32{2}))
	})
})

var _ = Describe("Attaching to running processes", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New("/depot/some-id", fakeRunner)

		fakeRunner.WhenRunning(
			fake_command_runner.CommandSpec{
				Path: binPath("iomux-link"),
			},
			func(cmd *exec.Cmd) error {
				time.Sleep(100 * time.Millisecond)

				cmd.Stdout.Write([]byte("hi out\n"))

				time.Sleep(100 * time.Millisecond)

				cmd.Stderr.Write([]byte("hi err\n"))

				time.Sleep(100 * time.Millisecond)

				dummyCmd := exec.Command("/bin/bash", "-c", "exit 42")
				dummyCmd.Run()

				cmd.ProcessState = dummyCmd.ProcessState

				return nil
			},
		)
	})

	It("streams their stdout and stderr into the channel", func(done Done) {
		setupSuccessfulSpawn()

		processID, _, err := processTracker.Run(exec.Command("xxx"))
		Expect(err).NotTo(HaveOccurred())

		processStreamChannel, err := processTracker.Attach(processID)
		Expect(err).ToNot(HaveOccurred())

		chunk1 := <-processStreamChannel
		Expect(chunk1.Source).To(Equal(warden.ProcessStreamSourceStdout))
		Expect(string(chunk1.Data)).To(Equal("hi out\n"))
		Expect(chunk1.ExitStatus).To(BeNil())

		chunk2 := <-processStreamChannel
		Expect(chunk2.Source).To(Equal(warden.ProcessStreamSourceStderr))
		Expect(string(chunk2.Data)).To(Equal("hi err\n"))
		Expect(chunk2.ExitStatus).To(BeNil())

		close(done)
	}, 5.0)

	Context("when the process is not yet linked to", func() {
		It("runs iomux-link", func() {
			setupSuccessfulSpawn()

			processTracker.Restore(0)

			Expect(fakeRunner).ToNot(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: binPath("iomux-link"),
				},
			))

			_, err := processTracker.Attach(0)
			Expect(err).ToNot(HaveOccurred())

			Eventually(fakeRunner).Should(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: binPath("iomux-link"),
				},
			))
		})
	})

	Context("when the process completes", func() {
		It("yields the exit status and closes the channel", func(done Done) {
			setupSuccessfulSpawn()

			processID, _, err := processTracker.Run(exec.Command("xxx"))
			Expect(err).NotTo(HaveOccurred())

			processStreamChannel, err := processTracker.Attach(processID)
			Expect(err).ToNot(HaveOccurred())

			<-processStreamChannel
			<-processStreamChannel

			chunk3 := <-processStreamChannel
			Expect(chunk3.Source).To(BeZero())
			Expect(string(chunk3.Data)).To(Equal(""))
			Expect(chunk3.ExitStatus).ToNot(BeNil())
			Expect(*chunk3.ExitStatus).To(Equal(uint32(42)))

			_, ok := <-processStreamChannel
			Expect(ok).To(BeFalse(), "channel is not closed")

			close(done)
		}, 5.0)
	})
})

var _ = Describe("Unlinking active processes", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New("/depot/some-id", fakeRunner)
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

		_, _, err := processTracker.Run(exec.Command("xxx"))
		Expect(err).ToNot(HaveOccurred())

		_, _, err = processTracker.Run(exec.Command("xxx"))
		Expect(err).ToNot(HaveOccurred())

		Eventually(linked).Should(Receive())
		Eventually(linked).Should(Receive())

		processTracker.UnlinkAll()

		Expect(fakeRunner).To(HaveSignalled(
			fake_command_runner.CommandSpec{
				Path: "/depot/some-id/bin/iomux-link",
				Args: []string{
					"-w", "/depot/some-id/processes/0/cursors",
					"/depot/some-id/processes/0",
				},
			},
			os.Interrupt,
		))

		Expect(fakeRunner).To(HaveSignalled(
			fake_command_runner.CommandSpec{
				Path: "/depot/some-id/bin/iomux-link",
				Args: []string{
					"-w", "/depot/some-id/processes/1/cursors",
					"/depot/some-id/processes/1",
				},
			},
			os.Interrupt,
		))
	})
})

var _ = Describe("Listing active process IDs", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()
		processTracker = process_tracker.New("/depot/some-id", fakeRunner)
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

		processID1, _, err := processTracker.Run(exec.Command("xxx"))
		Expect(err).ToNot(HaveOccurred())

		processID2, _, err := processTracker.Run(exec.Command("xxx"))
		Expect(err).ToNot(HaveOccurred())

		runningIDs := append(<-running, <-running...)

		Expect(runningIDs).To(ContainElement(processID1))
		Expect(runningIDs).To(ContainElement(processID2))
	})
})
