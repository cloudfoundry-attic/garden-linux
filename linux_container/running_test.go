package linux_container_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os/exec"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	networkFakes "github.com/cloudfoundry-incubator/garden-linux/network/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/old/bandwidth_manager/fake_bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/port_pool/fake_port_pool"
	"github.com/cloudfoundry-incubator/garden-linux/old/quota_manager/fake_quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker/fake_process_tracker"
	wfakes "github.com/cloudfoundry-incubator/garden/fakes"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
)

var _ = Describe("Linux containers", func() {
	var fakeRunner *fake_command_runner.FakeCommandRunner
	var containerResources *linux_backend.Resources
	var container *linux_container.LinuxContainer
	var fakeProcessTracker *fake_process_tracker.FakeProcessTracker
	var containerDir string

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeProcessTracker = new(fake_process_tracker.FakeProcessTracker)

		var err error
		containerDir, err = ioutil.TempDir("", "depot")
		Expect(err).ToNot(HaveOccurred())

		_, subnet, _ := net.ParseCIDR("2.3.4.0/30")
		containerResources = linux_backend.NewResources(
			1234,
			1235,
			&linux_backend.Network{
				IP:     net.ParseIP("1.2.3.4"),
				Subnet: subnet,
			},
			"some-bridge",
			[]uint32{},
			nil,
		)
	})

	JustBeforeEach(func() {
		container = linux_container.NewLinuxContainer(
			lagertest.NewTestLogger("test"),
			"some-id",
			"some-handle",
			containerDir,
			nil,
			1*time.Second,
			containerResources,
			fake_port_pool.New(1000),
			fakeRunner,
			fake_cgroups_manager.New("/cgroups", "some-id"),
			fake_quota_manager.New(),
			fake_bandwidth_manager.New(),
			fakeProcessTracker,
			process.Env{"env1": "env1Value", "env2": "env2Value"},
			new(networkFakes.FakeFilter),
		)
	})

	Describe("Running", func() {
		It("runs the /bin/bash via wsh with the given script as the input, and rlimits in env", func() {
			_, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
				Args: []string{"arg1", "arg2"},
				Limits: garden.ResourceLimits{
					As:         uint64ptr(1),
					Core:       uint64ptr(2),
					Cpu:        uint64ptr(3),
					Data:       uint64ptr(4),
					Fsize:      uint64ptr(5),
					Locks:      uint64ptr(6),
					Memlock:    uint64ptr(7),
					Msgqueue:   uint64ptr(8),
					Nice:       uint64ptr(9),
					Nofile:     uint64ptr(10),
					Nproc:      uint64ptr(11),
					Rss:        uint64ptr(12),
					Rtprio:     uint64ptr(13),
					Sigpending: uint64ptr(14),
					Stack:      uint64ptr(15),
				},
			}, garden.ProcessIO{})

			Expect(err).ToNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Expect(ranCmd.Path).To(Equal(containerDir + "/bin/wsh"))

			Expect(ranCmd.Args).To(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
				"arg1",
				"arg2",
			}))

			Expect(ranCmd.Env).To(Equal([]string{
				"RLIMIT_AS=1",
				"RLIMIT_CORE=2",
				"RLIMIT_CPU=3",
				"RLIMIT_DATA=4",
				"RLIMIT_FSIZE=5",
				"RLIMIT_LOCKS=6",
				"RLIMIT_MEMLOCK=7",
				"RLIMIT_MSGQUEUE=8",
				"RLIMIT_NICE=9",
				"RLIMIT_NOFILE=10",
				"RLIMIT_NPROC=11",
				"RLIMIT_RSS=12",
				"RLIMIT_RTPRIO=13",
				"RLIMIT_SIGPENDING=14",
				"RLIMIT_STACK=15",
			}))
		})

		It("runs wsh with the --pidfile parameter and configures the Process with this pidfile", func() {
			_, err := container.Run(garden.ProcessSpec{
				User: "alice",
				Path: "/some/script",
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Expect(ranCmd.Args).To(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "alice",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
			}))
		})

		It("configures a signaller with the same pid as the pidfile parameter", func() {
			_, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, signaller := fakeProcessTracker.RunArgsForCall(0)
			Expect(signaller).To(Equal(&linux_backend.NamespacedSignaller{
				ContainerPath: containerDir,
				Runner:        fakeRunner,
				PidFilePath:   containerDir + "/processes/1.pid",
			}))
		})

		It("uses unique process IDs for each process", func() {
			_, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			_, err = container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			id1, _, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			id2, _, _, _, _ := fakeProcessTracker.RunArgsForCall(1)

			Expect(id1).ToNot(Equal(id2))
		})

		It("should return an error when an environment variable is malformed", func() {
			_, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
				Env:  []string{"a"},
			}, garden.ProcessIO{})
			Expect(err).To(MatchError(HavePrefix("process: malformed environment")))
		})

		It("runs the script with environment variables", func() {
			_, err := container.Run(garden.ProcessSpec{
				User: "bob",
				Path: "/some/script",
				Env:  []string{"ESCAPED=kurt \"russell\"", "UNESCAPED=isaac\nhayes"},
			}, garden.ProcessIO{})

			Expect(err).ToNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Expect(ranCmd.Args).To(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "bob",
				"--env", `ESCAPED=kurt "russell"`,
				"--env", "UNESCAPED=isaac\nhayes",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
			}))
		})

		It("runs the script with the environment variables from the run taking precedence over the container environment variables", func() {
			_, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
				Env: []string{
					"env1=overridden",
				},
			}, garden.ProcessIO{})

			Expect(err).ToNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Expect(ranCmd.Args).To(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "env1=overridden",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
			}))
		})

		It("runs the script with the working dir set if present", func() {
			_, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
				Dir:  "/some/dir",
			}, garden.ProcessIO{})

			Expect(err).ToNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Expect(ranCmd.Args).To(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--dir", "/some/dir",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
			}))
		})

		It("runs the script with a TTY if present", func() {
			ttySpec := &garden.TTYSpec{
				WindowSize: &garden.WindowSize{
					Columns: 123,
					Rows:    456,
				},
			}

			_, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
				TTY:  ttySpec,
			}, garden.ProcessIO{})

			Expect(err).ToNot(HaveOccurred())

			_, _, _, tty, _ := fakeProcessTracker.RunArgsForCall(0)
			Expect(tty).To(Equal(ttySpec))
		})

		Describe("streaming", func() {
			JustBeforeEach(func() {
				fakeProcessTracker.RunStub = func(processID uint32, cmd *exec.Cmd, io garden.ProcessIO, tty *garden.TTYSpec, _ process_tracker.Signaller) (garden.Process, error) {
					writing := new(sync.WaitGroup)
					writing.Add(1)

					go func() {
						defer writing.Done()
						defer GinkgoRecover()

						_, err := fmt.Fprintf(io.Stdout, "hi out\n")
						Expect(err).ToNot(HaveOccurred())

						_, err = fmt.Fprintf(io.Stderr, "hi err\n")
						Expect(err).ToNot(HaveOccurred())
					}()

					process := new(wfakes.FakeProcess)

					process.IDReturns(processID)

					process.WaitStub = func() (int, error) {
						writing.Wait()
						return 123, nil
					}

					return process, nil
				}
			})

			It("streams stderr and stdout and exit status", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "/some/script",
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(process.ID()).To(Equal(uint32(1)))

				Eventually(stdout).Should(gbytes.Say("hi out\n"))
				Eventually(stderr).Should(gbytes.Say("hi err\n"))

				Expect(process.Wait()).To(Equal(123))
			})
		})

		It("only sets the given rlimits", func() {
			_, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
				Limits: garden.ResourceLimits{
					As:      uint64ptr(1),
					Cpu:     uint64ptr(3),
					Fsize:   uint64ptr(5),
					Memlock: uint64ptr(7),
					Nice:    uint64ptr(9),
					Nproc:   uint64ptr(11),
					Rtprio:  uint64ptr(13),
					Stack:   uint64ptr(15),
				},
			}, garden.ProcessIO{})

			Expect(err).ToNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Expect(ranCmd.Path).To(Equal(containerDir + "/bin/wsh"))

			Expect(ranCmd.Args).To(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
			}))

			Expect(ranCmd.Env).To(Equal([]string{
				"RLIMIT_AS=1",
				"RLIMIT_CPU=3",
				"RLIMIT_FSIZE=5",
				"RLIMIT_MEMLOCK=7",
				"RLIMIT_NICE=9",
				"RLIMIT_NPROC=11",
				"RLIMIT_RTPRIO=13",
				"RLIMIT_STACK=15",
			}))
		})

		Context("when the user is not set", func() {
			It("returns an error", func() {
				_, err := container.Run(garden.ProcessSpec{
					Path: "whoami",
					Args: []string{},
				}, garden.ProcessIO{})
				Expect(err).To(MatchError(ContainSubstring("A User for the process to run as must be specified")))
			})
		})

		Context("when spawning fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeProcessTracker.RunReturns(nil, disaster)
			})

			It("returns the error", func() {
				_, err := container.Run(garden.ProcessSpec{
					Path: "/some/script",
					User: "root",
				}, garden.ProcessIO{})
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Attaching", func() {
		Context("to a started process", func() {
			JustBeforeEach(func() {
				fakeProcessTracker.AttachStub = func(id uint32, io garden.ProcessIO) (garden.Process, error) {
					writing := new(sync.WaitGroup)
					writing.Add(1)

					go func() {
						defer writing.Done()
						defer GinkgoRecover()

						_, err := fmt.Fprintf(io.Stdout, "hi out\n")
						Expect(err).ToNot(HaveOccurred())

						_, err = fmt.Fprintf(io.Stderr, "hi err\n")
						Expect(err).ToNot(HaveOccurred())
					}()

					process := new(wfakes.FakeProcess)

					process.IDReturns(42)

					process.WaitStub = func() (int, error) {
						writing.Wait()
						return 123, nil
					}

					return process, nil
				}
			})

			It("streams stderr and stdout and exit status", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				process, err := container.Attach(1, garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Expect(err).ToNot(HaveOccurred())

				pid, _ := fakeProcessTracker.AttachArgsForCall(0)
				Expect(pid).To(Equal(uint32(1)))

				Expect(process.ID()).To(Equal(uint32(42)))

				Eventually(stdout).Should(gbytes.Say("hi out\n"))
				Eventually(stderr).Should(gbytes.Say("hi err\n"))

				Expect(process.Wait()).To(Equal(123))
			})
		})

		Context("when attaching fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeProcessTracker.AttachReturns(nil, disaster)
			})

			It("returns the error", func() {
				_, err := container.Attach(42, garden.ProcessIO{})
				Expect(err).To(Equal(disaster))
			})
		})
	})

})

func uint64ptr(n uint64) *uint64 {
	return &n
}
