package container_daemon_test

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_runner"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ShellRunnerStep", func() {
	var runner *fake_runner.FakeRunner

	BeforeEach(func() {
		runner = new(fake_runner.FakeRunner)
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
			step := &ShellRunnerStep{Runner: runner, Path: path}
			err := step.Init()
			Expect(err).ToNot(HaveOccurred())
			Expect(runner.StartArgsForCall(0)).To(Equal(exec.Command("sh", path)))
		})

		It("returns error if fails to start a shell command", func() {
			runner.StartReturns(errors.New("what"))

			step := &ShellRunnerStep{Runner: runner, Path: path}
			err := step.Init()
			Expect(err).To(HaveOccurred())
		})

		It("returns error if fails shell command does not exit 0", func() {
			runner.WaitReturns(byte(1), nil)

			step := &ShellRunnerStep{Runner: runner, Path: path}
			err := step.Init()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when a given path does not exist", func() {
		It("does not execute a shell command", func() {
			step := &ShellRunnerStep{Runner: runner, Path: "/whatever.sh"}
			step.Init()
			Expect(runner.StartCallCount()).To(Equal(0))
		})

		It("does not return an error", func() {
			step := &ShellRunnerStep{Runner: runner, Path: "/whatever.sh"}
			Expect(step.Init()).To(Succeed())
		})
	})
})
