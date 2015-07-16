package containerizer_test

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"

	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ShellRunnerStep", func() {
	var runner *fake_command_runner.FakeCommandRunner

	BeforeEach(func() {
		runner = fake_command_runner.New()
	})

	Context("when a given path exists", func() {
		var path string

		BeforeEach(func() {
			tmpdir, err := ioutil.TempDir("", "")
			Expect(err).ToNot(HaveOccurred())

			path = filepath.Join(tmpdir, "whatever.sh")
			Expect(ioutil.WriteFile(path, []byte(""), 0700)).To(Succeed())
		})

		AfterEach(func() {
			if path != "" {
				os.RemoveAll(path)
			}
		})

		It("runs a shell command", func() {
			step := &containerizer.ShellRunnerStep{Runner: runner, Path: path}
			err := step.Run()
			Expect(err).ToNot(HaveOccurred())
			Expect(runner).To(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: "sh",
					Args: []string{path},
				},
			))
		})

		It("returns error if fails to start a shell command", func() {
			runner.WhenRunning(fake_command_runner.CommandSpec{}, func(*exec.Cmd) error {
				return errors.New("what")
			})

			step := &containerizer.ShellRunnerStep{Runner: runner, Path: path}
			err := step.Run()
			Expect(err).To(HaveOccurred())
		})

		It("returns error if shell command does not exit 0", func() {
			runner.WhenWaitingFor(fake_command_runner.CommandSpec{}, func(*exec.Cmd) error {
				return errors.New("booo")
			})

			step := &containerizer.ShellRunnerStep{Runner: runner, Path: path}
			err := step.Run()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when a given path does not exist", func() {
		It("does not execute a shell command", func() {
			step := &containerizer.ShellRunnerStep{Runner: runner, Path: "/whatever.sh"}
			step.Run()
			Expect(runner.StartedCommands()).To(HaveLen(0))
		})

		It("does not return an error", func() {
			step := &containerizer.ShellRunnerStep{Runner: runner, Path: "/whatever.sh"}
			Expect(step.Run()).To(Succeed())
		})
	})
})
