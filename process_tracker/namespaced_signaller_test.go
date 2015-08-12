package process_tracker_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden-linux/process_tracker"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var _ = Describe("Namespaced Signaller", func() {
	var (
		containerPath         string
		processId             uint32
		processPidFileContent string
		signaller             *process_tracker.NamespacedSignaller
		fakeRunner            *fake_command_runner.FakeCommandRunner
		request               *process_tracker.SignalRequest
	)

	BeforeEach(func() {
		var err error
		containerPath, err = ioutil.TempDir("", "namespacedsignaller")
		Expect(err).ToNot(HaveOccurred())

		err = os.Mkdir(filepath.Join(containerPath, "processes"), 0755)
		Expect(err).ToNot(HaveOccurred())

		fakeRunner = fake_command_runner.New()
		signaller = &process_tracker.NamespacedSignaller{
			Runner:        fakeRunner,
			ContainerPath: containerPath,
			Logger:        lagertest.NewTestLogger("test"),
			Timeout:       time.Millisecond * 100,
		}

		processId = 12345
		request = &process_tracker.SignalRequest{
			Pid:    processId,
			Link:   nil,
			Signal: syscall.SIGKILL,
		}
	})

	AfterEach(func() {
		os.RemoveAll(containerPath)
	})

	JustBeforeEach(func() {
		processPidFile := filepath.Join(containerPath, "processes", fmt.Sprintf("%d.pid", processId))
		Expect(ioutil.WriteFile(processPidFile, []byte(processPidFileContent), 0755)).To(Succeed())
	})

	Context("when the pidfile exists", func() {
		BeforeEach(func() {
			processPidFileContent = " 12345\n"
		})

		It("kills a process using ./bin/wsh based on its pid", func() {
			Expect(signaller.Signal(request)).To(Succeed())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: filepath.Join(containerPath, "bin/wsh"),
					Args: []string{
						"--socket", filepath.Join(containerPath, "run/wshd.sock"),
						"--user", "root",
						"kill", "-9", "12345",
					},
				}))
		})
	})

	Context("when the pidfile is not present", func() {
		JustBeforeEach(func() {
			os.RemoveAll(containerPath)
		})

		It("returns an appropriate error", func() {
			errMsg := fmt.Sprintf("linux_backend: can't open PID file: open %s/processes/%d.pid: no such file or directory", containerPath, processId)
			Expect(signaller.Signal(request)).To(MatchError(errMsg))
		})
	})

	Context("when the pidfile is empty", func() {
		BeforeEach(func() {
			processPidFileContent = ""
		})

		It("returns an appropriate error", func() {
			Expect(signaller.Signal(request)).To(MatchError("linux_backend: can't read PID file: is empty or non existent"))
		})
	})

	Context("when the pidfile does not contain a number", func() {
		BeforeEach(func() {
			processPidFileContent = "not-a-pid\n"
		})

		It("returns an appropriate error", func() {
			Expect(signaller.Signal(request)).To(MatchError("linux_backend: can't parse PID file content: expected integer"))
		})
	})
})
