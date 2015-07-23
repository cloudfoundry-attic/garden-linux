package container_daemon_test

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/docker/docker/pkg/reexec"

	"io/ioutil"

	"path"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	_ "github.com/cloudfoundry-incubator/garden-linux/container_daemon/proc_starter"
)

func init() {
	if reexec.Init() {
		os.Exit(0)
	}
}

type FakeCommandRunner struct {
}

func (r *FakeCommandRunner) Start(cmd *exec.Cmd) error {
	return cmd.Start()
}

func (r *FakeCommandRunner) Wait(cmd *exec.Cmd) byte {
	return exitStatusFromErr(cmd.Wait())
}

type FakeProcessSignaller struct {
}

func (s *FakeProcessSignaller) Signal(pid int, signal syscall.Signal) error {
	return syscall.Kill(pid, signal)
}

var _ = Describe("wsh and daemon integration", func() {
	var daemon *container_daemon.ContainerDaemon
	var tempDir string
	var socketPath string
	var wsh string

	BeforeEach(func() {
		var err error
		wsh, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/container_daemon/wsh")
		Expect(err).ToNot(HaveOccurred())

		tempDir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		socketPath = path.Join(tempDir, "test.sock")
		listener, err := unix_socket.NewListenerFromPath(socketPath)
		Expect(err).ToNot(HaveOccurred())

		daemon = &container_daemon.ContainerDaemon{
			CmdPreparer: &container_daemon.ProcessSpecPreparer{
				Users:   container_daemon.LibContainerUser{},
				Reexec:  container_daemon.CommandFunc(reexec.Command),
				Rlimits: &container_daemon.RlimitsManager{},
			},
			Spawner: &container_daemon.Spawn{
				Runner: &FakeCommandRunner{},
			},
			Signaller: &FakeProcessSignaller{},
		}

		go func(listener container_daemon.Listener) {
			defer GinkgoRecover()
			Expect(daemon.Run(listener)).To(Succeed())
		}(listener)
	})

	It("should run a program", func() {
		wshCmd := exec.Command(wsh,
			"--socket", socketPath,
			"--user", "root",
			"echo", "hello")

		op, err := wshCmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred())
		Expect(string(op)).To(Equal("hello\n"))
	})

	It("should avoid a race condition when sending a kill signal", func(done Done) {
		for i := 0; i < 20; i++ {
			wshCmd := exec.Command(wsh,
				"--socket", socketPath,
				"--user", "root",
				"sh", "-c",
				`while true; do echo -n "x"; sleep 1; done`)

			err := wshCmd.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(syscall.Kill(wshCmd.Process.Pid, syscall.SIGKILL)).To(Succeed())
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatusFromErr(wshCmd.Wait())).To(Equal(byte(255)))
		}
		close(done)
	}, 40.0)

	It("receives the correct exit status and output from a process which is sent SIGTERM", func(done Done) {
		stdout := gbytes.NewBuffer()

		wshCmd := exec.Command(wsh,
			"--socket", socketPath,
			"--user", "root",
			"sh", "-c", `
				  trap 'echo termed; exit 142' TERM
					while true; do
					  echo waiting
					  sleep 1
					done
				`)
		wshCmd.Stdout = stdout
		wshCmd.Stderr = GinkgoWriter

		err := wshCmd.Start()
		Expect(err).ToNot(HaveOccurred())

		Eventually(stdout, "10s").Should(gbytes.Say("waiting"))
		Expect(syscall.Kill(wshCmd.Process.Pid, syscall.SIGTERM)).To(Succeed())

		Expect(exitStatusFromErr(wshCmd.Wait())).To(Equal(byte(142)))
		Eventually(stdout, "2s").Should(gbytes.Say("termed"))

		close(done)
	}, 320.0)

	It("receives the correct exit status and output from a process exits 255", func(done Done) {
		for i := 0; i < 200; i++ {
			stdout := gbytes.NewBuffer()

			wshCmd := exec.Command(wsh,
				"--socket", socketPath,
				"--user", "root",
				"sh", "-c", `
					for i in $(seq 0 512); do
					  echo 0123456789
					done

					echo ended
					exit 255
				`)
			wshCmd.Stdout = stdout
			wshCmd.Stderr = GinkgoWriter

			err := wshCmd.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(exitStatusFromErr(wshCmd.Wait())).To(Equal(byte(255)))
			Eventually(stdout, "2s").Should(gbytes.Say("ended"))
		}
		close(done)
	}, 320.0)

	It("applies the provided rlimits", func() {
		wshCmd := exec.Command(wsh,
			"--socket", socketPath,
			"--user", "root",
			"sh", "-c",
			"ulimit -n")

		wshCmd.Env = append(wshCmd.Env, "RLIMIT_NOFILE=16")

		op, err := wshCmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred())
		Expect(string(op)).To(Equal("16\n"))
	})
})

func exitStatusFromErr(err error) byte {
	if exitError, ok := err.(*exec.ExitError); ok {
		waitStatus := exitError.Sys().(syscall.WaitStatus)
		return byte(waitStatus.ExitStatus())
	} else if err != nil {
		println("exitStatusFromErr found error", err)
		return container_daemon.UnknownExitStatus
	} else {
		return 0
	}
}
