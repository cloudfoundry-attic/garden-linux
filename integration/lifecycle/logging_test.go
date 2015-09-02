package lifecycle_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

type CmdRunner struct {
	Cmd *exec.Cmd
}

func (r *CmdRunner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	defer GinkgoRecover()

	allOutput := gbytes.NewBuffer()
	session, err := gexec.Start(
		r.Cmd,
		io.MultiWriter(allOutput, r.Cmd.Stdout),
		io.MultiWriter(allOutput, r.Cmd.Stderr),
	)
	Ω(err).ShouldNot(HaveOccurred())
	detectStartCheck := allOutput.Detect("garden-linux.started")

	for {
		select {
		case <-detectStartCheck: // works even with empty string
			allOutput.CancelDetects()
			detectStartCheck = nil
			close(ready)

		case signal := <-signals:
			session.Signal(signal)

		case <-session.Exited:
			if session.ExitCode() == 0 {
				return nil
			}

			return fmt.Errorf("exit status %d", session.ExitCode())
		}
	}
}

type LogRunnerCreator struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (l *LogRunnerCreator) Create(cmd *exec.Cmd) ifrit.Runner {
	cmd.Stdout = l.Stdout
	cmd.Stderr = l.Stderr
	return &CmdRunner{Cmd: cmd}
}

var _ = Describe("Logging", func() {
	var container garden.Container
	var containerSpec garden.ContainerSpec
	var stdout *gbytes.Buffer

	BeforeEach(func() {
		containerSpec = garden.ContainerSpec{}
	})

	JustBeforeEach(func() {
		var err error
		stdout = gbytes.NewBuffer()

		creator := &LogRunnerCreator{
			Stdout: io.MultiWriter(stdout, GinkgoWriter),
			Stderr: io.MultiWriter(stdout, GinkgoWriter),
		}
		client = startGardenWithRunnerCreator(creator)
		container, err = client.Create(containerSpec)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when container is created", func() {
		BeforeEach(func() {
			containerSpec = garden.ContainerSpec{
				Env: []string{"PASSWORD=MY_SECRET"},
			}
		})

		It("should not log any environment variables", func() {
			Expect(stdout).ToNot(gbytes.Say("PASSWORD"))
			Expect(stdout).ToNot(gbytes.Say("MY_SECRET"))
		})
	})

	Context("when container spawn a new process", func() {
		It("should not log any environment variables and command line arguments", func() {
			process, err := container.Run(garden.ProcessSpec{
				User: "alice",
				Path: "echo",
				Args: []string{"-username", "banana"},
				Env:  []string{"PASSWORD=MY_SECRET"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			Expect(stdout).ToNot(gbytes.Say("PASSWORD"))
			Expect(stdout).ToNot(gbytes.Say("MY_SECRET"))
			Expect(stdout).ToNot(gbytes.Say("-username"))
			Expect(stdout).ToNot(gbytes.Say("banana"))
		})
	})

})
