package linux_container_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
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

	var oldLang string

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeProcessTracker = new(fake_process_tracker.FakeProcessTracker)

		var err error
		containerDir, err = ioutil.TempDir("", "depot")
		Ω(err).ShouldNot(HaveOccurred())

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

		oldLang = os.Getenv("LANG")
		os.Setenv("LANG", "en_US.UTF-8")
	})

	AfterEach(func() {
		if oldLang == "" {
			os.Unsetenv("LANG")
		} else {
			os.Setenv("LANG", oldLang)
		}
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

			Ω(err).ShouldNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Path).Should(Equal(containerDir + "/bin/wsh"))

			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "LANG=en_US.UTF-8",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
				"arg1",
				"arg2",
			}))

			Ω(ranCmd.Env).Should(Equal([]string{
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
				Path: "/some/script",
			}, garden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "LANG=en_US.UTF-8",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
			}))
		})

		It("configures a signaller with the same pid as the pidfile parameter", func() {
			_, err := container.Run(garden.ProcessSpec{
				Path: "/some/script",
			}, garden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			_, _, _, _, signaller := fakeProcessTracker.RunArgsForCall(0)
			Ω(signaller).Should(Equal(&linux_backend.NamespacedSignaller{
				ContainerPath: containerDir,
				Runner:        fakeRunner,
				PidFilePath:   containerDir + "/processes/1.pid",
			}))
		})

		It("uses unique process IDs for each process", func() {
			_, err := container.Run(garden.ProcessSpec{
				Path: "/some/script",
			}, garden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			_, err = container.Run(garden.ProcessSpec{
				Path: "/some/script",
			}, garden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			id1, _, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			id2, _, _, _, _ := fakeProcessTracker.RunArgsForCall(1)

			Ω(id1).ShouldNot(Equal(id2))
		})

		It("should return an error when an environment variable is malformed", func() {
			_, err := container.Run(garden.ProcessSpec{
				Path: "/some/script",
				Env:  []string{"a"},
			}, garden.ProcessIO{})
			Ω(err).Should(MatchError(HavePrefix("process: malformed environment")))
		})

		It("runs the script with environment variables", func() {
			_, err := container.Run(garden.ProcessSpec{
				Path: "/some/script",
				Env:  []string{"ESCAPED=kurt \"russell\"", "UNESCAPED=isaac\nhayes"},
			}, garden.ProcessIO{})

			Ω(err).ShouldNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", `ESCAPED=kurt "russell"`,
				"--env", "LANG=en_US.UTF-8",
				"--env", "UNESCAPED=isaac\nhayes",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
			}))
		})

		Describe("LANG environment variable", func() {
			It("forwards the LANG variable the environment if the user doesn't specify it", func() {
				os.Setenv("LANG", "C")

				_, err := container.Run(garden.ProcessSpec{
					Path: "/some/script",
				}, garden.ProcessIO{})
				Ω(err).ShouldNot(HaveOccurred())

				_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
				Ω(ranCmd.Args).Should(ContainElement("LANG=C"))
			})

			It("forwards the LANG variable the environment if the user doesn't specify it", func() {
				os.Unsetenv("LANG")

				_, err := container.Run(garden.ProcessSpec{
					Path: "/some/script",
				}, garden.ProcessIO{})
				Ω(err).ShouldNot(HaveOccurred())

				_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
				Ω(ranCmd.Args).Should(ContainElement("LANG=en_US.UTF-8"))
			})
		})

		It("runs the script with the environment variables from the run taking precedence over the container environment variables", func() {
			_, err := container.Run(garden.ProcessSpec{
				Path: "/some/script",
				Env: []string{
					"env1=overridden",
					"LANG=POSIX",
				},
			}, garden.ProcessIO{})

			Ω(err).ShouldNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "LANG=POSIX",
				"--env", "env1=overridden",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
			}))
		})

		It("runs the script with the working dir set if present", func() {
			_, err := container.Run(garden.ProcessSpec{
				Path: "/some/script",
				Dir:  "/some/dir",
			}, garden.ProcessIO{})

			Ω(err).ShouldNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "LANG=en_US.UTF-8",
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
				Path: "/some/script",
				TTY:  ttySpec,
			}, garden.ProcessIO{})

			Ω(err).ShouldNot(HaveOccurred())

			_, _, _, tty, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(tty).Should(Equal(ttySpec))
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
						Ω(err).ShouldNot(HaveOccurred())

						_, err = fmt.Fprintf(io.Stderr, "hi err\n")
						Ω(err).ShouldNot(HaveOccurred())
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
					Path: "/some/script",
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(process.ID()).Should(Equal(uint32(1)))

				Eventually(stdout).Should(gbytes.Say("hi out\n"))
				Eventually(stderr).Should(gbytes.Say("hi err\n"))

				Ω(process.Wait()).Should(Equal(123))
			})
		})

		It("only sets the given rlimits", func() {
			_, err := container.Run(garden.ProcessSpec{
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

			Ω(err).ShouldNot(HaveOccurred())

			_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Path).Should(Equal(containerDir + "/bin/wsh"))

			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "LANG=en_US.UTF-8",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--pidfile", containerDir + "/processes/1.pid",
				"/some/script",
			}))

			Ω(ranCmd.Env).Should(Equal([]string{
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

		Context("with 'privileged' true", func() {
			Context("when the user flag is empty", func() {
				It("runs with --user root", func() {
					_, err := container.Run(garden.ProcessSpec{
						Path:       "/some/script",
						Privileged: true,
					}, garden.ProcessIO{})

					Ω(err).ToNot(HaveOccurred())

					_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
					Ω(ranCmd.Path).Should(Equal(containerDir + "/bin/wsh"))

					Ω(ranCmd.Args).Should(Equal([]string{
						containerDir + "/bin/wsh",
						"--socket", containerDir + "/run/wshd.sock",
						"--user", "root",
						"--env", "LANG=en_US.UTF-8",
						"--env", "env1=env1Value",
						"--env", "env2=env2Value",
						"--pidfile", containerDir + "/processes/1.pid",
						"/some/script",
					}))
				})
			})

			Context("when the user flag is specified", func() {
				It("runs with --user set to the specified user", func() {
					_, err := container.Run(garden.ProcessSpec{
						Path:       "/some/script",
						Privileged: true,
						User:       "potato",
					}, garden.ProcessIO{})

					Ω(err).ToNot(HaveOccurred())

					_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
					Ω(ranCmd.Path).Should(Equal(containerDir + "/bin/wsh"))

					Ω(ranCmd.Args).Should(Equal([]string{
						containerDir + "/bin/wsh",
						"--socket", containerDir + "/run/wshd.sock",
						"--user", "potato",
						"--env", "LANG=en_US.UTF-8",
						"--env", "env1=env1Value",
						"--env", "env2=env2Value",
						"--pidfile", containerDir + "/processes/1.pid",
						"/some/script",
					}))
				})
			})
		})

		Context("with 'privileged' false", func() {
			Context("when the user flag is empty", func() {
				It("runs with --user vcap", func() {
					_, err := container.Run(garden.ProcessSpec{
						Path:       "/some/script",
						Privileged: false,
					}, garden.ProcessIO{})

					Ω(err).ToNot(HaveOccurred())

					_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
					Ω(ranCmd.Path).Should(Equal(containerDir + "/bin/wsh"))

					Ω(ranCmd.Args).Should(Equal([]string{
						containerDir + "/bin/wsh",
						"--socket", containerDir + "/run/wshd.sock",
						"--user", "vcap",
						"--env", "LANG=en_US.UTF-8",
						"--env", "env1=env1Value",
						"--env", "env2=env2Value",
						"--pidfile", containerDir + "/processes/1.pid",
						"/some/script",
					}))
				})
			})

			Context("when the user flag is specified", func() {
				It("runs with --user set to the specified user", func() {
					_, err := container.Run(garden.ProcessSpec{
						Path:       "/some/script",
						Privileged: true,
						User:       "potato",
					}, garden.ProcessIO{})

					Ω(err).ToNot(HaveOccurred())

					_, ranCmd, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
					Ω(ranCmd.Path).Should(Equal(containerDir + "/bin/wsh"))

					Ω(ranCmd.Args).Should(Equal([]string{
						containerDir + "/bin/wsh",
						"--socket", containerDir + "/run/wshd.sock",
						"--user", "potato",
						"--env", "LANG=en_US.UTF-8",
						"--env", "env1=env1Value",
						"--env", "env2=env2Value",
						"--pidfile", containerDir + "/processes/1.pid",
						"/some/script",
					}))
				})
			})
		})

		Context("when spawning fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeProcessTracker.RunReturns(nil, disaster)
			})

			It("returns the error", func() {
				_, err := container.Run(garden.ProcessSpec{
					Path:       "/some/script",
					Privileged: true,
				}, garden.ProcessIO{})
				Ω(err).Should(Equal(disaster))
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
						Ω(err).ShouldNot(HaveOccurred())

						_, err = fmt.Fprintf(io.Stderr, "hi err\n")
						Ω(err).ShouldNot(HaveOccurred())
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
				Ω(err).ShouldNot(HaveOccurred())

				pid, _ := fakeProcessTracker.AttachArgsForCall(0)
				Ω(pid).Should(Equal(uint32(1)))

				Ω(process.ID()).Should(Equal(uint32(42)))

				Eventually(stdout).Should(gbytes.Say("hi out\n"))
				Eventually(stderr).Should(gbytes.Say("hi err\n"))

				Ω(process.Wait()).Should(Equal(123))
			})
		})

		Context("when attaching fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeProcessTracker.AttachReturns(nil, disaster)
			})

			It("returns the error", func() {
				_, err := container.Attach(42, garden.ProcessIO{})
				Ω(err).Should(Equal(disaster))
			})
		})
	})

})

func uint64ptr(n uint64) *uint64 {
	return &n
}
