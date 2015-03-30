package linux_backend_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var _ = Describe("Namespaced Signaller", func() {
	It("kills a process using ./bin/wsh based on its pid", func() {
		tmp, err := ioutil.TempDir("", "namespacedsignaller")
		Ω(err).ShouldNot(HaveOccurred())
		defer os.RemoveAll(tmp)

		pidFile := filepath.Join(tmp, "thepid.file")

		fakeRunner := fake_command_runner.New()
		signaller := &linux_backend.NamespacedSignaller{
			Runner:        fakeRunner,
			ContainerPath: "/fish/finger",
			PidFilePath:   pidFile,
		}

		Ω(ioutil.WriteFile(pidFile, []byte(" 12345\n"), 0755)).Should(Succeed())

		Ω(signaller.Signal(os.Kill)).Should(Succeed())
		Ω(fakeRunner).Should(HaveExecutedSerially(
			fake_command_runner.CommandSpec{
				Path: "/fish/finger/bin/wsh",
				Args: []string{
					"--socket", "/fish/finger/run/wshd.sock",
					"kill", "-9", "12345",
				},
			}))
	})

	It("returns an appropriate error when the pidfile is not present", func() {
		fakeRunner := fake_command_runner.New()
		signaller := &linux_backend.NamespacedSignaller{
			Runner:        fakeRunner,
			ContainerPath: "/fish/finger",
			PidFilePath:   "/does/not/exist",
		}

		Ω(signaller.Signal(os.Kill)).Should(MatchError("linux_backend: namespaced-signaller can't open PID file: open /does/not/exist: no such file or directory"))
	})

	It("returns an appropriate error when the pidfile is empty", func() {
		tmp, err := ioutil.TempDir("", "namespacedsignaller")
		Ω(err).ShouldNot(HaveOccurred())
		defer os.RemoveAll(tmp)

		pidFile := filepath.Join(tmp, "thepid.file")

		fakeRunner := fake_command_runner.New()
		signaller := &linux_backend.NamespacedSignaller{
			Runner:        fakeRunner,
			ContainerPath: "/fish/finger",
			PidFilePath:   pidFile,
		}

		Ω(ioutil.WriteFile(pidFile, []byte(""), 0755)).Should(Succeed())

		Ω(signaller.Signal(os.Kill)).Should(MatchError("namespaced-signaller: can't read pidfile: is empty or non existant"))
	})

	It("returns an appropriate error when the pidfile does not contain a number", func() {
		tmp, err := ioutil.TempDir("", "namespacedsignaller")
		Ω(err).ShouldNot(HaveOccurred())
		defer os.RemoveAll(tmp)

		pidFile := filepath.Join(tmp, "thepid.file")

		fakeRunner := fake_command_runner.New()
		signaller := &linux_backend.NamespacedSignaller{
			Runner:        fakeRunner,
			ContainerPath: "/fish/finger",
			PidFilePath:   pidFile,
		}

		Ω(ioutil.WriteFile(pidFile, []byte("not-a-pid\n"), 0755)).Should(Succeed())

		Ω(signaller.Signal(os.Kill)).Should(MatchError("namespaced-signaller: can't parse pidfile content: expected integer"))
	})
})
