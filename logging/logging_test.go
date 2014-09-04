package logging_test

import (
	"bytes"
	"os/exec"
	"time"

	. "github.com/cloudfoundry-incubator/warden-linux/logging"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Logging", func() {
	var innerRunner command_runner.CommandRunner
	var logger *lagertest.TestLogger

	var runner *Runner

	BeforeEach(func() {
		innerRunner = linux_command_runner.New()
		logger = lagertest.NewTestLogger("test")
	})

	JustBeforeEach(func() {
		runner = &Runner{
			CommandRunner: innerRunner,
			Logger:        logger,
		}
	})

	It("logs the duration it took to run the command", func() {
		err := runner.Run(exec.Command("sleep", "1"))
		Ω(err).ShouldNot(HaveOccurred())

		Ω(logger.TestSink.Logs()).Should(HaveLen(2))

		log := logger.TestSink.Logs()[1]

		took := log.Data["took"].(string)
		Ω(took).ShouldNot(BeEmpty())

		duration, err := time.ParseDuration(took)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(duration).Should(BeNumerically(">=", 1*time.Second))
	})

	It("logs the command's argv", func() {
		err := runner.Run(exec.Command("bash", "-c", "echo sup"))
		Ω(err).ShouldNot(HaveOccurred())

		Ω(logger.TestSink.Logs()).Should(HaveLen(2))

		log := logger.TestSink.Logs()[0]
		Ω(log.LogLevel).Should(Equal(lager.DEBUG))
		Ω(log.Message).Should(Equal("test.command.starting"))
		Ω(log.Data["argv"]).Should(Equal([]interface{}{"bash", "-c", "echo sup"}))

		log = logger.TestSink.Logs()[1]
		Ω(log.LogLevel).Should(Equal(lager.DEBUG))
		Ω(log.Message).Should(Equal("test.command.succeeded"))
		Ω(log.Data["argv"]).Should(Equal([]interface{}{"bash", "-c", "echo sup"}))
	})

	Describe("running a command that exits normally", func() {
		It("logs its exit status with 'debug' level", func() {
			err := runner.Run(exec.Command("true"))
			Ω(err).ShouldNot(HaveOccurred())

			Ω(logger.TestSink.Logs()).Should(HaveLen(2))

			log := logger.TestSink.Logs()[1]
			Ω(log.LogLevel).Should(Equal(lager.DEBUG))
			Ω(log.Message).Should(Equal("test.command.succeeded"))
			Ω(log.Data["exit-status"]).Should(Equal(float64(0))) // JSOOOOOOOOOOOOOOOOOOON
		})

		Context("when the command has output to stdout/stderr", func() {
			It("does not log stdout/stderr", func() {
				err := runner.Run(exec.Command("sh", "-c", "echo hi out; echo hi err >&2"))
				Ω(err).ShouldNot(HaveOccurred())

				Ω(logger.TestSink.Logs()).Should(HaveLen(2))

				log := logger.TestSink.Logs()[1]
				Ω(log.LogLevel).Should(Equal(lager.DEBUG))
				Ω(log.Message).Should(Equal("test.command.succeeded"))
				Ω(log.Data).ShouldNot(HaveKey("stdout"))
				Ω(log.Data).ShouldNot(HaveKey("stderr"))
			})
		})
	})

	Describe("delegation", func() {
		var fakeRunner *fake_command_runner.FakeCommandRunner

		BeforeEach(func() {
			fakeRunner = fake_command_runner.New()
			innerRunner = fakeRunner
		})

		It("runs using the provided runner", func() {
			err := runner.Run(exec.Command("morgan-freeman"))
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "morgan-freeman",
			}))
		})
	})

	Describe("running a bogus command", func() {
		It("logs the error", func() {
			err := runner.Run(exec.Command("morgan-freeman"))
			Ω(err).Should(HaveOccurred())

			Ω(logger.TestSink.Logs()).Should(HaveLen(2))

			log := logger.TestSink.Logs()[1]
			Ω(log.LogLevel).Should(Equal(lager.ERROR))
			Ω(log.Message).Should(Equal("test.command.failed"))
			Ω(log.Data["error"]).ShouldNot(BeEmpty())
			Ω(log.Data).ShouldNot(HaveKey("exit-status"))
		})
	})

	Describe("running a command that exits nonzero", func() {
		It("logs its status with 'error' level", func() {
			err := runner.Run(exec.Command("false"))
			Ω(err).Should(HaveOccurred())

			Ω(logger.TestSink.Logs()).Should(HaveLen(2))

			log := logger.TestSink.Logs()[1]
			Ω(log.LogLevel).Should(Equal(lager.ERROR))
			Ω(log.Message).Should(Equal("test.command.failed"))
			Ω(log.Data["error"]).Should(Equal("exit status 1"))
			Ω(log.Data["exit-status"]).Should(Equal(float64(1))) // JSOOOOOOOOOOOOOOOOOOON
		})

		Context("when the command has output to stdout/stderr", func() {
			It("reports the stdout/stderr in the log data", func() {
				err := runner.Run(exec.Command("sh", "-c", "echo hi out; echo hi err >&2; exit 1"))
				Ω(err).Should(HaveOccurred())

				Ω(logger.TestSink.Logs()).Should(HaveLen(2))

				log := logger.TestSink.Logs()[1]
				Ω(log.LogLevel).Should(Equal(lager.ERROR))
				Ω(log.Message).Should(Equal("test.command.failed"))
				Ω(log.Data["stdout"]).Should(Equal("hi out\n"))
				Ω(log.Data["stderr"]).Should(Equal("hi err\n"))
			})

			Context("and it is being collected by the caller", func() {
				It("multiplexes to the caller and the logs", func() {
					stdout := new(bytes.Buffer)
					stderr := new(bytes.Buffer)

					cmd := exec.Command("sh", "-c", "echo hi out; echo hi err >&2; exit 1")
					cmd.Stdout = stdout
					cmd.Stderr = stderr

					err := runner.Run(cmd)
					Ω(err).Should(HaveOccurred())

					Ω(logger.TestSink.Logs()).Should(HaveLen(2))

					log := logger.TestSink.Logs()[1]
					Ω(log.LogLevel).Should(Equal(lager.ERROR))
					Ω(log.Message).Should(Equal("test.command.failed"))
					Ω(log.Data["stdout"]).Should(Equal("hi out\n"))
					Ω(log.Data["stderr"]).Should(Equal("hi err\n"))

					Ω(stdout.String()).Should(Equal("hi out\n"))
					Ω(stderr.String()).Should(Equal("hi err\n"))
				})
			})
		})
	})
})
