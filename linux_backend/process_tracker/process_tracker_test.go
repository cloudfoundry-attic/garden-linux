package process_tracker_test

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/process_tracker"
	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
)

var processTracker process_tracker.ProcessTracker
var tmpdir string

var _ = BeforeEach(func() {
	var err error

	tmpdir, err = ioutil.TempDir("", "process-tracker-tests")
	Ω(err).ShouldNot(HaveOccurred())

	iodaemon, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/iodaemon")
	Ω(err).ShouldNot(HaveOccurred())

	err = os.MkdirAll(filepath.Join(tmpdir, "bin"), 0755)
	Ω(err).ShouldNot(HaveOccurred())

	err = os.Rename(iodaemon, filepath.Join(tmpdir, "bin", "iodaemon"))
	Ω(err).ShouldNot(HaveOccurred())
})

var _ = AfterEach(func() {
	os.RemoveAll(tmpdir)
})

var _ = Describe("Running processes", func() {
	BeforeEach(func() {
		processTracker = process_tracker.New(tmpdir, linux_command_runner.New())
	})

	It("runs the process and returns its exit code", func() {
		cmd := exec.Command("bash", "-c", "exit 42")

		process, err := processTracker.Run(cmd, warden.ProcessIO{}, nil)
		Expect(err).NotTo(HaveOccurred())

		Ω(process.Wait()).Should(Equal(42))
	})

	It("returns unique process IDs", func() {
		process1, err := processTracker.Run(exec.Command("/bin/echo"), warden.ProcessIO{}, nil)
		Expect(err).NotTo(HaveOccurred())

		process2, err := processTracker.Run(exec.Command("/bin/date"), warden.ProcessIO{}, nil)
		Expect(err).NotTo(HaveOccurred())

		Ω(process1.ID()).ShouldNot(Equal(process2.ID()))
	})

	It("streams the process's stdout and stderr", func() {
		cmd := exec.Command(
			"/bin/bash",
			"-c",
			"echo 'hi out' && echo 'hi err' >&2",
		)

		stdout := gbytes.NewBuffer()
		stderr := gbytes.NewBuffer()

		_, err := processTracker.Run(cmd, warden.ProcessIO{
			Stdout: stdout,
			Stderr: stderr,
		}, nil)
		Expect(err).NotTo(HaveOccurred())

		Eventually(stdout).Should(gbytes.Say("hi out\n"))
		Eventually(stderr).Should(gbytes.Say("hi err\n"))
	})

	It("streams input to the process", func() {
		stdout := gbytes.NewBuffer()

		_, err := processTracker.Run(exec.Command("cat"), warden.ProcessIO{
			Stdin:  bytes.NewBufferString("stdin-line1\nstdin-line2\n"),
			Stdout: stdout,
		}, nil)
		Expect(err).NotTo(HaveOccurred())

		Eventually(stdout).Should(gbytes.Say("stdin-line1\nstdin-line2\n"))
	})

	Context("when there is an error reading the stdin stream", func() {
		It("does not close the process's stdin", func() {
			pipeR, pipeW := io.Pipe()
			stdout := gbytes.NewBuffer()

			process, err := processTracker.Run(exec.Command("cat"), warden.ProcessIO{
				Stdin:  pipeR,
				Stdout: stdout,
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			pipeW.Write([]byte("Hello stdin!"))
			Eventually(stdout).Should(gbytes.Say("Hello stdin!"))

			pipeW.CloseWithError(errors.New("Failed"))
			Consistently(stdout, 0.1).ShouldNot(gbytes.Say("."))

			pipeR, pipeW = io.Pipe()
			processTracker.Attach(process.ID(), warden.ProcessIO{
				Stdin: pipeR,
			})

			pipeW.Write([]byte("Hello again, stdin!"))
			Eventually(stdout).Should(gbytes.Say("Hello again, stdin!"))

			pipeW.Close()
			Ω(process.Wait()).Should(Equal(0))
		})
	})

	Context("with a tty", func() {
		It("forwards TTY signals to the process", func() {
			cmd := exec.Command("/bin/bash", "-c", `
				stty size
				trap "stty size; exit 123" SIGWINCH
				read
			`)

			stdout := gbytes.NewBuffer()

			process, err := processTracker.Run(cmd, warden.ProcessIO{
				Stdout: stdout,
			}, &warden.TTYSpec{
				WindowSize: &warden.WindowSize{
					Columns: 95,
					Rows:    13,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("13 95"))

			process.SetTTY(warden.TTYSpec{
				WindowSize: &warden.WindowSize{
					Columns: 101,
					Rows:    27,
				},
			})

			Eventually(stdout).Should(gbytes.Say("27 101"))
			Ω(process.Wait()).Should(Equal(123))
		})

		Describe("when a window size is not specified", func() {
			It("picks a default window size", func() {
				cmd := exec.Command("/bin/bash", "-c", `
					stty size
				`)

				stdout := gbytes.NewBuffer()

				_, err := processTracker.Run(cmd, warden.ProcessIO{
					Stdout: stdout,
				}, &warden.TTYSpec{})
				Expect(err).NotTo(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("24 80"))
			})
		})
	})

	Context("when spawning fails", func() {
		It("returns the error", func() {
			_, err := processTracker.Run(exec.Command("/bin/does-not-exist"), warden.ProcessIO{}, nil)
			Ω(err).Should(HaveOccurred())
		})
	})
})

var _ = Describe("Restoring processes", func() {
	BeforeEach(func() {
		processTracker = process_tracker.New(tmpdir, linux_command_runner.New())
	})

	It("makes the next process ID be higher than the highest restored ID", func() {
		processTracker.Restore(0)

		process, err := processTracker.Run(exec.Command("date"), warden.ProcessIO{}, nil)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.ID()).Should(Equal(uint32(1)))

		processTracker.Restore(5)

		process, err = processTracker.Run(exec.Command("date"), warden.ProcessIO{}, nil)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.ID()).Should(Equal(uint32(6)))
	})

	It("tracks the restored process", func() {
		processTracker.Restore(2)

		activeProcesses := processTracker.ActiveProcesses()
		Ω(activeProcesses).Should(HaveLen(1))
		Ω(activeProcesses[0].ID()).Should(Equal(uint32(2)))
	})
})

var _ = Describe("Attaching to running processes", func() {
	BeforeEach(func() {
		processTracker = process_tracker.New(tmpdir, linux_command_runner.New())
	})

	It("streams stdout, stdin, and stderr", func() {
		cmd := exec.Command("bash", "-c", `
			stuff=$(cat)
			echo "hi stdout" $stuff
			echo "hi stderr" $stuff >&2
		`)

		process, err := processTracker.Run(cmd, warden.ProcessIO{}, nil)
		Expect(err).NotTo(HaveOccurred())

		stdout := gbytes.NewBuffer()
		stderr := gbytes.NewBuffer()

		process, err = processTracker.Attach(process.ID(), warden.ProcessIO{
			Stdin:  bytes.NewBufferString("this-is-stdin"),
			Stdout: stdout,
			Stderr: stderr,
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(stdout).Should(gbytes.Say("hi stdout this-is-stdin"))
		Eventually(stderr).Should(gbytes.Say("hi stderr this-is-stdin"))
	})
})

var _ = Describe("Listing active process IDs", func() {
	BeforeEach(func() {
		processTracker = process_tracker.New(tmpdir, linux_command_runner.New())
	})

	It("includes running process IDs", func() {
		stdin1, stdinWriter1 := io.Pipe()
		stdin2, stdinWriter2 := io.Pipe()

		Ω(processTracker.ActiveProcesses()).Should(BeEmpty())

		process1, err := processTracker.Run(exec.Command("cat"), warden.ProcessIO{
			Stdin: stdin1,
		}, nil)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(processTracker.ActiveProcesses).Should(ConsistOf(process1))

		process2, err := processTracker.Run(exec.Command("cat"), warden.ProcessIO{
			Stdin: stdin2,
		}, nil)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(processTracker.ActiveProcesses).Should(ConsistOf(process1, process2))

		stdinWriter1.Close()
		Eventually(processTracker.ActiveProcesses).Should(ConsistOf(process2))

		stdinWriter2.Close()
		Eventually(processTracker.ActiveProcesses).Should(BeEmpty())
	})
})
