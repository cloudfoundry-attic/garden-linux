package container_daemon_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"code.cloudfoundry.org/garden-linux/container_daemon"
	"code.cloudfoundry.org/garden-linux/iodaemon/link"
	"github.com/docker/docker/pkg/reexec"

	"io/ioutil"

	"path"

	"code.cloudfoundry.org/garden-linux/container_daemon/unix_socket"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	_ "code.cloudfoundry.org/garden-linux/container_daemon/proc_starter"
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
		wsh, err = gexec.Build("code.cloudfoundry.org/garden-linux/container_daemon/wsh")
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

		go func(listener container_daemon.Listener, daemon *container_daemon.ContainerDaemon) {
			defer GinkgoRecover()
			Expect(daemon.Run(listener)).To(Succeed())
		}(listener, daemon)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("should avoid pinning the cpu when doing nothing", func() {
		stdout := gbytes.NewBuffer()

		wshCmd := exec.Command(wsh,
			"--socket", socketPath,
			"--user", "root",
			"sh", "-c", "echo 'hello'; sleep 2")
		wshCmd.Stdout = stdout

		err := wshCmd.Start()
		Expect(err).NotTo(HaveOccurred())

		// this is required to warm up the process such that full cpu
		// load can be evaluated in the subsequent command
		Eventually(stdout, "10s").Should(gbytes.Say("hello"))

		pid := wshCmd.Process.Pid
		output, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "%cpu=").CombinedOutput()
		Expect(err).NotTo(HaveOccurred())

		percentageCpu, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 32)
		Expect(err).NotTo(HaveOccurred())

		Expect(percentageCpu).To(BeNumerically("<", 30))
		wshCmd.Wait()
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
				"--readSignals",
				"sh", "-c",
				`while true; do echo -n "x"; sleep 1; done`)
			wshReader, wshWriter, err := os.Pipe()
			Expect(err).ToNot(HaveOccurred())
			wshCmd.ExtraFiles = []*os.File{wshReader}

			err = wshCmd.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(sendSignal(wshWriter, syscall.SIGKILL)).To(Succeed())
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatusFromErr(wshCmd.Wait())).To(Equal(byte(255)))
		}
		close(done)
	}, 40.0)

	It("receives the correct exit status and output from a process which is sent SIGTERM", func() {
		stdout := gbytes.NewBuffer()

		wshCmd := exec.Command(wsh,
			"--socket", socketPath,
			"--user", "root",
			"--readSignals",
			"sh", "-c", `
				  trap 'echo termed; exit 142' TERM
					while true; do
					  echo waiting
					  sleep 1
					done
				`)

		wshReader, wshWriter, err := os.Pipe()
		Expect(err).ToNot(HaveOccurred())
		wshCmd.ExtraFiles = []*os.File{wshReader}

		wshCmd.Stdout = stdout
		wshCmd.Stderr = GinkgoWriter

		err = wshCmd.Start()
		Expect(err).ToNot(HaveOccurred())

		Eventually(stdout, "15s").Should(gbytes.Say("waiting"))
		Expect(sendSignal(wshWriter, syscall.SIGTERM)).To(Succeed())

		Expect(exitStatusFromErr(wshCmd.Wait())).To(Equal(byte(142)))
		Eventually(stdout, "2s").Should(gbytes.Say("termed"))
	})

	It("receives the correct exit status and output from a process exits 255", func(done Done) {
		for i := 0; i < 20; i++ {
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
			Expect(wshCmd.Start()).To(Succeed())

			Expect(exitStatusFromErr(wshCmd.Wait())).To(Equal(byte(255)))
			Eventually(stdout, "3s").Should(gbytes.Say("ended"))
		}
		close(done)
	}, 120.0)

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

func sendSignal(wshWriter io.Writer, signal syscall.Signal) error {
	data, err := json.Marshal(&link.SignalMsg{Signal: signal})
	if err != nil {
		return err
	}
	_, err = wshWriter.Write(data)
	return err
}

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
