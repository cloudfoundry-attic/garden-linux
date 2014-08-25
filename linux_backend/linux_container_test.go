package linux_backend_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/bandwidth_manager/fake_bandwidth_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/network_pool"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/port_pool/fake_port_pool"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/process_tracker"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/process_tracker/fake_process_tracker"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/quota_manager/fake_quota_manager"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var fakeCgroups *fake_cgroups_manager.FakeCgroupsManager
var fakeQuotaManager *fake_quota_manager.FakeQuotaManager
var fakeBandwidthManager *fake_bandwidth_manager.FakeBandwidthManager
var fakeRunner *fake_command_runner.FakeCommandRunner
var containerResources *linux_backend.Resources
var container *linux_backend.LinuxContainer
var fakePortPool *fake_port_pool.FakePortPool
var fakeProcessTracker *fake_process_tracker.FakeProcessTracker
var containerDir string

var _ = Describe("Linux containers", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeCgroups = fake_cgroups_manager.New("/cgroups", "some-id")

		fakeQuotaManager = fake_quota_manager.New()
		fakeBandwidthManager = fake_bandwidth_manager.New()
		fakeProcessTracker = new(fake_process_tracker.FakeProcessTracker)

		_, ipNet, err := net.ParseCIDR("10.254.0.0/24")
		Ω(err).ShouldNot(HaveOccurred())

		fakePortPool = fake_port_pool.New(1000)

		networkPool := network_pool.New(ipNet)

		network, err := networkPool.Acquire()
		Ω(err).ShouldNot(HaveOccurred())

		containerDir, err = ioutil.TempDir("", "depot")
		Ω(err).ShouldNot(HaveOccurred())

		err = os.Mkdir(filepath.Join(containerDir, "run"), 0755)
		Ω(err).ShouldNot(HaveOccurred())
		err = ioutil.WriteFile(filepath.Join(containerDir, "run", "wshd.pid"), []byte("12345\n"), 0644)
		Ω(err).ShouldNot(HaveOccurred())

		containerResources = linux_backend.NewResources(
			1234,
			network,
			[]uint32{},
		)

		container = linux_backend.NewLinuxContainer(
			lagertest.NewTestLogger("test"),
			"some-id",
			"some-handle",
			containerDir,
			map[string]string{
				"property-name": "property-value",
			},
			1*time.Second,
			containerResources,
			fakePortPool,
			fakeRunner,
			fakeCgroups,
			fakeQuotaManager,
			fakeBandwidthManager,
			fakeProcessTracker,
			[]string{"env1=env1Value", "env2=env2Value"},
		)
	})

	Describe("Snapshotting", func() {
		memoryLimits := warden.MemoryLimits{
			LimitInBytes: 1,
		}

		diskLimits := warden.DiskLimits{
			BlockSoft: 3,
			BlockHard: 4,

			InodeSoft: 13,
			InodeHard: 14,

			ByteSoft: 23,
			ByteHard: 24,
		}

		bandwidthLimits := warden.BandwidthLimits{
			RateInBytesPerSecond:      1,
			BurstRateInBytesPerSecond: 2,
		}

		cpuLimits := warden.CPULimits{
			LimitInShares: 1,
		}

		BeforeEach(func() {
			var err error

			err = container.Start()
			Ω(err).ShouldNot(HaveOccurred())

			_, _, err = container.NetIn(1, 2)
			Ω(err).ShouldNot(HaveOccurred())

			_, _, err = container.NetIn(3, 4)
			Ω(err).ShouldNot(HaveOccurred())

			err = container.NetOut("network-a", 1)
			Ω(err).ShouldNot(HaveOccurred())

			err = container.NetOut("network-b", 2)
			Ω(err).ShouldNot(HaveOccurred())

			p1 := new(fake_process_tracker.FakeLinuxProcess)
			p1.IDReturns(1)
			p1.WithTTYReturns(true)

			p2 := new(fake_process_tracker.FakeLinuxProcess)
			p2.IDReturns(2)
			p2.WithTTYReturns(false)

			p3 := new(fake_process_tracker.FakeLinuxProcess)
			p3.IDReturns(3)
			p3.WithTTYReturns(true)

			fakeProcessTracker.ActiveProcessesReturns([]process_tracker.LinuxProcess{p1, p2, p3})
		})

		It("writes a JSON ContainerSnapshot", func() {
			out := new(bytes.Buffer)

			err := container.Snapshot(out)
			Ω(err).ShouldNot(HaveOccurred())

			var snapshot linux_backend.ContainerSnapshot

			err = json.NewDecoder(out).Decode(&snapshot)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(snapshot.ID).Should(Equal("some-id"))
			Ω(snapshot.Handle).Should(Equal("some-handle"))

			Ω(snapshot.GraceTime).Should(Equal(1 * time.Second))

			Ω(snapshot.State).Should(Equal("active"))

			Ω(snapshot.Resources).Should(Equal(
				linux_backend.ResourcesSnapshot{
					UID:     containerResources.UID,
					Network: containerResources.Network,
					Ports:   containerResources.Ports,
				},
			))

			Ω(snapshot.NetIns).Should(Equal(
				[]linux_backend.NetInSpec{
					{
						HostPort:      1,
						ContainerPort: 2,
					},
					{
						HostPort:      3,
						ContainerPort: 4,
					},
				},
			))

			Ω(snapshot.NetOuts).Should(Equal(
				[]linux_backend.NetOutSpec{
					{
						Network: "network-a",
						Port:    1,
					},
					{
						Network: "network-b",
						Port:    2,
					},
				},
			))

			Ω(snapshot.Processes).Should(ContainElement(
				linux_backend.ProcessSnapshot{
					ID:  1,
					TTY: true,
				},
			))

			Ω(snapshot.Processes).Should(ContainElement(
				linux_backend.ProcessSnapshot{
					ID:  2,
					TTY: false,
				},
			))

			Ω(snapshot.Processes).Should(ContainElement(
				linux_backend.ProcessSnapshot{
					ID:  3,
					TTY: true,
				},
			))

			Ω(snapshot.Properties).Should(Equal(warden.Properties(map[string]string{
				"property-name": "property-value",
			})))

			Ω(snapshot.EnvVars).Should(Equal([]string{"env1=env1Value", "env2=env2Value"}))
		})

		Context("with limits set", func() {
			BeforeEach(func() {
				err := container.LimitMemory(memoryLimits)
				Ω(err).ShouldNot(HaveOccurred())

				// oom exits immediately since it's faked out; should see event,
				// and it should show up in the snapshot
				Eventually(container.Events).Should(ContainElement("out of memory"))

				err = container.LimitDisk(diskLimits)
				Ω(err).ShouldNot(HaveOccurred())

				err = container.LimitBandwidth(bandwidthLimits)
				Ω(err).ShouldNot(HaveOccurred())

				err = container.LimitCPU(cpuLimits)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("saves them", func() {
				out := new(bytes.Buffer)

				err := container.Snapshot(out)
				Ω(err).ShouldNot(HaveOccurred())

				var snapshot linux_backend.ContainerSnapshot

				err = json.NewDecoder(out).Decode(&snapshot)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(snapshot.State).Should(Equal("stopped"))
				Ω(snapshot.Events).Should(Equal([]string{"out of memory"}))

				Ω(snapshot.Limits).Should(Equal(
					linux_backend.LimitsSnapshot{
						Memory:    &memoryLimits,
						Disk:      &diskLimits,
						Bandwidth: &bandwidthLimits,
						CPU:       &cpuLimits,
					},
				))
			})
		})

		Context("with no limits set", func() {
			It("saves them as nil, not zero values", func() {
				out := new(bytes.Buffer)

				err := container.Snapshot(out)
				Ω(err).ShouldNot(HaveOccurred())

				var snapshot linux_backend.ContainerSnapshot

				err = json.NewDecoder(out).Decode(&snapshot)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(snapshot.Limits).Should(Equal(
					linux_backend.LimitsSnapshot{
						Memory:    nil,
						Disk:      nil,
						Bandwidth: nil,
						CPU:       nil,
					},
				))

			})
		})
	})

	Describe("Restoring", func() {
		It("sets the container's state and events", func() {
			err := container.Restore(linux_backend.ContainerSnapshot{
				State:  "active",
				Events: []string{"out of memory", "foo"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.State()).Should(Equal(linux_backend.State("active")))
			Ω(container.Events()).Should(Equal([]string{
				"out of memory",
				"foo",
			}))

		})

		It("restores process state", func() {
			err := container.Restore(linux_backend.ContainerSnapshot{
				State:  "active",
				Events: []string{},

				Processes: []linux_backend.ProcessSnapshot{
					{
						ID:  0,
						TTY: false,
					},
					{
						ID:  1,
						TTY: true,
					},
				},
			})
			Ω(err).ShouldNot(HaveOccurred())

			pid, tty := fakeProcessTracker.RestoreArgsForCall(0)
			Ω(pid).Should(Equal(uint32(0)))
			Ω(tty).Should(BeFalse())

			pid, tty = fakeProcessTracker.RestoreArgsForCall(1)
			Ω(pid).Should(Equal(uint32(1)))
			Ω(tty).Should(BeTrue())
		})

		It("restores environment variables", func() {
			err := container.Restore(linux_backend.ContainerSnapshot{
				EnvVars: []string{"env1=env1value", "env2=env2Value"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.CurrentEnvVars()).Should(Equal([]string{"env1=env1value", "env2=env2Value"}))

		})

		It("redoes network setup and net-in/net-outs", func() {
			err := container.Restore(linux_backend.ContainerSnapshot{
				State:  "active",
				Events: []string{},

				NetIns: []linux_backend.NetInSpec{
					{
						HostPort:      1234,
						ContainerPort: 5678,
					},
					{
						HostPort:      1235,
						ContainerPort: 5679,
					},
				},

				NetOuts: []linux_backend.NetOutSpec{
					{
						Network: "somehost.example.com",
						Port:    80,
					},
					{
						Network: "someotherhost.example.com",
						Port:    8080,
					},
				},
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/net.sh",
					Args: []string{"setup"},
				},
				fake_command_runner.CommandSpec{
					Path: containerDir + "/net.sh",
					Args: []string{"in"},
				},
				fake_command_runner.CommandSpec{
					Path: containerDir + "/net.sh",
					Args: []string{"in"},
				},
				fake_command_runner.CommandSpec{
					Path: containerDir + "/net.sh",
					Args: []string{"out"},
				},
				fake_command_runner.CommandSpec{
					Path: containerDir + "/net.sh",
					Args: []string{"out"},
				},
			))
		})

		for _, cmd := range []string{"setup", "in", "out"} {
			command := cmd

			Context("when net.sh "+cmd+" fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRunner.WhenRunning(
						fake_command_runner.CommandSpec{
							Path: containerDir + "/net.sh",
							Args: []string{command},
						}, func(*exec.Cmd) error {
							return disaster
						},
					)
				})

				It("returns the error", func() {
					err := container.Restore(linux_backend.ContainerSnapshot{
						State:  "active",
						Events: []string{},

						NetIns: []linux_backend.NetInSpec{
							{
								HostPort:      1234,
								ContainerPort: 5678,
							},
							{
								HostPort:      1235,
								ContainerPort: 5679,
							},
						},

						NetOuts: []linux_backend.NetOutSpec{
							{
								Network: "somehost.example.com",
								Port:    80,
							},
							{
								Network: "someotherhost.example.com",
								Port:    8080,
							},
						},
					})
					Ω(err).Should(Equal(disaster))
				})
			})
		}

		It("re-enforces the memory limit", func() {
			err := container.Restore(linux_backend.ContainerSnapshot{
				State:  "active",
				Events: []string{},

				Limits: linux_backend.LimitsSnapshot{
					Memory: &warden.MemoryLimits{
						LimitInBytes: 1024,
					},
				},
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeCgroups.SetValues()).Should(ContainElement(
				fake_cgroups_manager.SetValue{
					Subsystem: "memory",
					Name:      "memory.limit_in_bytes",
					Value:     "1024",
				},
			))

			Ω(fakeCgroups.SetValues()).Should(ContainElement(
				fake_cgroups_manager.SetValue{
					Subsystem: "memory",
					Name:      "memory.memsw.limit_in_bytes",
					Value:     "1024",
				},
			))

			// oom will exit immediately as the command runner is faked out
			Eventually(container.Events).Should(ContainElement("out of memory"))
		})

		Context("when no memory limit is present", func() {
			It("does not set a limit", func() {
				err := container.Restore(linux_backend.ContainerSnapshot{
					State:  "active",
					Events: []string{},
				})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeCgroups.SetValues()).Should(BeEmpty())
			})
		})

		Context("when re-enforcing the memory limit fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeCgroups.WhenSetting("memory", "memory.limit_in_bytes", func() error {
					return disaster
				})
			})

			It("returns the error", func() {
				err := container.Restore(linux_backend.ContainerSnapshot{
					State:  "active",
					Events: []string{},

					Limits: linux_backend.LimitsSnapshot{
						Memory: &warden.MemoryLimits{
							LimitInBytes: 1024,
						},
					},
				})
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Starting", func() {
		It("executes the container's start.sh with the correct environment", func() {
			err := container.Start()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/start.sh",
					Env: []string{
						"id=some-id",
						"container_iface_mtu=1500",
						"PATH=" + os.Getenv("PATH"),
					},
				},
			))

		})

		It("changes the container's state to active", func() {
			Ω(container.State()).Should(Equal(linux_backend.StateBorn))

			err := container.Start()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.State()).Should(Equal(linux_backend.StateActive))
		})

		Context("when start.sh fails", func() {
			nastyError := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/start.sh",
					}, func(*exec.Cmd) error {
						return nastyError
					},
				)
			})

			It("returns the error", func() {
				err := container.Start()
				Ω(err).Should(Equal(nastyError))
			})

			It("does not change the container's state", func() {
				Ω(container.State()).Should(Equal(linux_backend.StateBorn))

				err := container.Start()
				Ω(err).Should(HaveOccurred())

				Ω(container.State()).Should(Equal(linux_backend.StateBorn))
			})
		})
	})

	Describe("Stopping", func() {
		It("executes the container's stop.sh", func() {
			err := container.Stop(false)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/stop.sh",
				},
			))

		})

		It("sets the container's state to stopped", func() {
			Ω(container.State()).Should(Equal(linux_backend.StateBorn))

			err := container.Stop(false)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.State()).Should(Equal(linux_backend.StateStopped))

		})

		Context("when kill is true", func() {
			It("executes stop.sh with -w 0", func() {
				err := container.Stop(true)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/stop.sh",
						Args: []string{"-w", "0"},
					},
				))

			})
		})

		Context("when stop.sh fails", func() {
			nastyError := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/stop.sh",
					}, func(*exec.Cmd) error {
						return nastyError
					},
				)
			})

			It("returns the error", func() {
				err := container.Stop(false)
				Ω(err).Should(Equal(nastyError))
			})

			It("does not change the container's state", func() {
				Ω(container.State()).Should(Equal(linux_backend.StateBorn))

				err := container.Stop(false)
				Ω(err).Should(HaveOccurred())

				Ω(container.State()).Should(Equal(linux_backend.StateBorn))
			})
		})

		Context("when the container has an oom notifier running", func() {
			BeforeEach(func() {
				err := container.LimitMemory(warden.MemoryLimits{
					LimitInBytes: 42,
				})

				Ω(err).ShouldNot(HaveOccurred())
			})

			It("stops it", func() {
				err := container.Stop(false)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveKilled(fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/oom",
				}))

			})
		})
	})

	Describe("Cleaning up", func() {
		It("unlinks from all running processes", func() {
			container.Cleanup()

			Ω(fakeProcessTracker.UnlinkAllCallCount()).Should(Equal(1))
		})

		Context("when the container has an oom notifier running", func() {
			BeforeEach(func() {
				err := container.LimitMemory(warden.MemoryLimits{
					LimitInBytes: 42,
				})

				Ω(err).ShouldNot(HaveOccurred())
			})

			It("stops it", func() {
				container.Cleanup()

				Ω(fakeRunner).Should(HaveKilled(fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/oom",
				}))

			})
		})
	})

	Describe("Streaming data in", func() {
		var source io.Reader

		BeforeEach(func() {
			source = strings.NewReader("the-tar-content")
		})

		It("streams the input to tar xf in the container", func() {
			fakeRunner.WhenRunning(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/nstar",
					Args: []string{
						"12345",
						"vcap",
						"/some/directory/dst",
					},
				},
				func(cmd *exec.Cmd) error {
					bytes, err := ioutil.ReadAll(cmd.Stdin)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(string(bytes)).Should(Equal("the-tar-content"))

					return nil
				},
			)

			err := container.StreamIn("/some/directory/dst", bytes.NewBufferString("the-tar-content"))
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when tar fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/bin/nstar",
					},
					func(cmd *exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				err := container.StreamIn("/some/directory/dst", nil)
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Streaming out", func() {
		var destination *bytes.Buffer

		BeforeEach(func() {
			destination = new(bytes.Buffer)
		})

		It("streams the output of tar cf to the destination", func() {
			fakeRunner.WhenRunning(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/nstar",
					Args: []string{
						"12345",
						"vcap",
						"/some/directory",
						"dst",
					},
				},
				func(cmd *exec.Cmd) error {
					_, err := cmd.Stdout.Write([]byte("the-compressed-content"))
					Ω(err).ShouldNot(HaveOccurred())

					return nil
				},
			)

			reader, err := container.StreamOut("/some/directory/dst")
			Ω(err).ShouldNot(HaveOccurred())

			bytes, err := ioutil.ReadAll(reader)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(bytes)).Should(Equal("the-compressed-content"))
		})

		It("closes the server-side dupe of of the pipe's write end", func() {
			var outPipe io.Writer

			fakeRunner.WhenRunning(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/nstar",
					Args: []string{
						"12345",
						"vcap",
						"/some/directory",
						"dst",
					},
				},
				func(cmd *exec.Cmd) error {
					outPipe = cmd.Stdout
					return nil
				},
			)

			_, err := container.StreamOut("/some/directory/dst")
			Ω(err).ShouldNot(HaveOccurred())

			Ω(outPipe).ShouldNot(BeNil())

			_, err = outPipe.Write([]byte("sup"))
			Ω(err).Should(HaveOccurred())
		})

		Context("when there's a trailing slash", func() {
			It("compresses the directory's contents", func() {
				_, err := container.StreamOut("/some/directory/dst/")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveBackgrounded(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/bin/nstar",
						Args: []string{
							"12345",
							"vcap",
							"/some/directory/dst/",
							".",
						},
					},
				))
			})
		})

		Context("when executing the command fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/bin/nstar",
					}, func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				_, err := container.StreamOut("/some/dst")
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Running", func() {
		It("runs the /bin/bash via wsh with the given script as the input, and rlimits in env", func() {
			_, err := container.Run(warden.ProcessSpec{
				Path: "/some/script",
				Args: []string{"arg1", "arg2"},
				Limits: warden.ResourceLimits{
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
			}, warden.ProcessIO{})

			Ω(err).ShouldNot(HaveOccurred())

			ranCmd, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Path).Should(Equal(containerDir + "/bin/wsh"))

			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
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

		It("runs the script with environment variables", func() {
			_, err := container.Run(warden.ProcessSpec{
				Path: "/some/script",
				Env:  []string{"ESCAPED=kurt \"russell\"", "UNESCAPED=isaac\nhayes"},
			}, warden.ProcessIO{})

			Ω(err).ShouldNot(HaveOccurred())

			ranCmd, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--env", `ESCAPED=kurt "russell"`,
				"--env", "UNESCAPED=isaac\nhayes",
				"/some/script",
			}))
		})

		It("runs the script with the working dir set if present", func() {
			_, err := container.Run(warden.ProcessSpec{
				Path: "/some/script",
				Dir:  "/some/dir",
			}, warden.ProcessIO{})

			Ω(err).ShouldNot(HaveOccurred())

			ranCmd, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
				"--dir", "/some/dir",
				"/some/script",
			}))
		})

		It("runs the script with a TTY if present", func() {
			ttySpec := &warden.TTYSpec{
				WindowSize: &warden.WindowSize{
					Columns: 123,
					Rows:    456,
				},
			}

			_, err := container.Run(warden.ProcessSpec{
				Path: "/some/script",
				TTY:  ttySpec,
			}, warden.ProcessIO{})

			Ω(err).ShouldNot(HaveOccurred())

			_, _, tty := fakeProcessTracker.RunArgsForCall(0)
			Ω(tty).Should(Equal(ttySpec))
		})

		Describe("streaming", func() {
			BeforeEach(func() {
				fakeProcessTracker.RunStub = func(cmd *exec.Cmd, io warden.ProcessIO, tty *warden.TTYSpec) (process_tracker.LinuxProcess, error) {
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

					process := new(fake_process_tracker.FakeLinuxProcess)

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

				process, err := container.Run(warden.ProcessSpec{
					Path: "/some/script",
				}, warden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(process.ID()).Should(Equal(uint32(42)))

				Eventually(stdout).Should(gbytes.Say("hi out\n"))
				Eventually(stderr).Should(gbytes.Say("hi err\n"))

				Ω(process.Wait()).Should(Equal(123))
			})
		})

		It("only sets the given rlimits", func() {
			_, err := container.Run(warden.ProcessSpec{
				Path: "/some/script",
				Limits: warden.ResourceLimits{
					As:      uint64ptr(1),
					Cpu:     uint64ptr(3),
					Fsize:   uint64ptr(5),
					Memlock: uint64ptr(7),
					Nice:    uint64ptr(9),
					Nproc:   uint64ptr(11),
					Rtprio:  uint64ptr(13),
					Stack:   uint64ptr(15),
				},
			}, warden.ProcessIO{})

			Ω(err).ShouldNot(HaveOccurred())

			ranCmd, _, _ := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Path).Should(Equal(containerDir + "/bin/wsh"))

			Ω(ranCmd.Args).Should(Equal([]string{
				containerDir + "/bin/wsh",
				"--socket", containerDir + "/run/wshd.sock",
				"--user", "vcap",
				"--env", "env1=env1Value",
				"--env", "env2=env2Value",
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
			It("runs with --user root", func() {
				_, err := container.Run(warden.ProcessSpec{
					Path:       "/some/script",
					Privileged: true,
				}, warden.ProcessIO{})

				Ω(err).ToNot(HaveOccurred())

				ranCmd, _, _ := fakeProcessTracker.RunArgsForCall(0)
				Ω(ranCmd.Path).Should(Equal(containerDir + "/bin/wsh"))

				Ω(ranCmd.Args).Should(Equal([]string{
					containerDir + "/bin/wsh",
					"--socket", containerDir + "/run/wshd.sock",
					"--user", "root",
					"--env", "env1=env1Value",
					"--env", "env2=env2Value",
					"/some/script",
				}))
			})
		})

		Context("when spawning fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeProcessTracker.RunReturns(nil, disaster)
			})

			It("returns the error", func() {
				_, err := container.Run(warden.ProcessSpec{
					Path:       "/some/script",
					Privileged: true,
				}, warden.ProcessIO{})
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Attaching", func() {
		Context("to a started process", func() {
			BeforeEach(func() {
				fakeProcessTracker.AttachStub = func(id uint32, io warden.ProcessIO) (process_tracker.LinuxProcess, error) {
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

					process := new(fake_process_tracker.FakeLinuxProcess)

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

				process, err := container.Attach(1, warden.ProcessIO{
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

			BeforeEach(func() {
				fakeProcessTracker.AttachReturns(nil, disaster)
			})

			It("returns the error", func() {
				_, err := container.Attach(42, warden.ProcessIO{})
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Limiting bandwidth", func() {
		limits := warden.BandwidthLimits{
			RateInBytesPerSecond:      128,
			BurstRateInBytesPerSecond: 256,
		}

		It("sets the limit via the bandwidth manager with the new limits", func() {
			err := container.LimitBandwidth(limits)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeBandwidthManager.EnforcedLimits).Should(ContainElement(limits))
		})

		Context("when setting the limit fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeBandwidthManager.SetLimitsError = disaster
			})

			It("returns the error", func() {
				err := container.LimitBandwidth(limits)
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Getting the current bandwidth limit", func() {
		limits := warden.BandwidthLimits{
			RateInBytesPerSecond:      128,
			BurstRateInBytesPerSecond: 256,
		}

		It("returns a zero value if no limits are set", func() {
			receivedLimits, err := container.CurrentBandwidthLimits()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(receivedLimits).Should(BeZero())
		})

		Context("when limits are set", func() {
			It("returns them", func() {
				err := container.LimitBandwidth(limits)
				Ω(err).ShouldNot(HaveOccurred())

				receivedLimits, err := container.CurrentBandwidthLimits()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(receivedLimits).Should(Equal(limits))
			})

			Context("when limits fail to be set", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeBandwidthManager.SetLimitsError = disaster
				})

				It("does not update the current limits", func() {
					err := container.LimitBandwidth(limits)
					Ω(err).Should(Equal(disaster))

					receivedLimits, err := container.CurrentBandwidthLimits()
					Ω(err).ShouldNot(HaveOccurred())
					Ω(receivedLimits).Should(BeZero())
				})
			})
		})
	})

	Describe("Limiting memory", func() {
		It("starts the oom notifier", func() {
			limits := warden.MemoryLimits{
				LimitInBytes: 102400,
			}

			err := container.LimitMemory(limits)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/oom",
					Args: []string{"/cgroups/memory/instance-some-id"},
				},
			))

		})

		It("sets memory.limit_in_bytes and then memory.memsw.limit_in_bytes", func() {
			limits := warden.MemoryLimits{
				LimitInBytes: 102400,
			}

			err := container.LimitMemory(limits)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeCgroups.SetValues()).Should(Equal(
				[]fake_cgroups_manager.SetValue{
					{
						Subsystem: "memory",
						Name:      "memory.limit_in_bytes",
						Value:     "102400",
					},
					{
						Subsystem: "memory",
						Name:      "memory.memsw.limit_in_bytes",
						Value:     "102400",
					},
					{
						Subsystem: "memory",
						Name:      "memory.limit_in_bytes",
						Value:     "102400",
					},
				},
			))

		})

		Context("when the oom notifier is already running", func() {
			It("does not start another", func() {
				started := 0

				fakeRunner.WhenRunning(fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/oom",
				}, func(*exec.Cmd) error {
					started++
					return nil
				})

				limits := warden.MemoryLimits{
					LimitInBytes: 102400,
				}

				err := container.LimitMemory(limits)
				Ω(err).ShouldNot(HaveOccurred())

				err = container.LimitMemory(limits)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(started).Should(Equal(1))
			})
		})

		Context("when the oom notifier exits 0", func() {
			BeforeEach(func() {
				fakeRunner.WhenWaitingFor(fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/oom",
				}, func(cmd *exec.Cmd) error {
					return nil
				})
			})

			It("stops the container", func() {
				limits := warden.MemoryLimits{
					LimitInBytes: 102400,
				}

				err := container.LimitMemory(limits)
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(fakeRunner).Should(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/stop.sh",
					},
				))
			})

			It("registers an 'out of memory' event", func() {
				limits := warden.MemoryLimits{
					LimitInBytes: 102400,
				}

				err := container.LimitMemory(limits)
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(func() []string {
					return container.Events()
				}).Should(ContainElement("out of memory"))
			})
		})

		Context("when setting memory.memsw.limit_in_bytes fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeCgroups.WhenSetting("memory", "memory.memsw.limit_in_bytes", func() error {
					return disaster
				})
			})

			It("does not fail", func() {
				err := container.LimitMemory(warden.MemoryLimits{
					LimitInBytes: 102400,
				})

				Ω(err).ShouldNot(HaveOccurred())
			})
		})

		Context("when setting memory.limit_in_bytes fails only the first time", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				numSet := 0

				fakeCgroups.WhenSetting("memory", "memory.limit_in_bytes", func() error {
					numSet++

					if numSet == 1 {
						return disaster
					}

					return nil
				})
			})

			It("succeeds", func() {
				fakeCgroups.WhenGetting("memory", "memory.limit_in_bytes", func() (string, error) {
					return "123", nil
				})

				err := container.LimitMemory(warden.MemoryLimits{
					LimitInBytes: 102400,
				})

				Ω(err).ShouldNot(HaveOccurred())
			})
		})

		Context("when setting memory.limit_in_bytes fails the second time", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				numSet := 0

				fakeCgroups.WhenSetting("memory", "memory.limit_in_bytes", func() error {
					numSet++

					if numSet == 2 {
						return disaster
					}

					return nil
				})
			})

			It("returns the error and no limits", func() {
				err := container.LimitMemory(warden.MemoryLimits{
					LimitInBytes: 102400,
				})

				Ω(err).Should(Equal(disaster))
			})
		})

		Context("when starting the oom notifier fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/oom",
				}, func(cmd *exec.Cmd) error {
					return disaster
				})
			})

			It("returns the error", func() {
				err := container.LimitMemory(warden.MemoryLimits{
					LimitInBytes: 102400,
				})

				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Getting the current memory limit", func() {
		It("returns the limited memory", func() {
			fakeCgroups.WhenGetting("memory", "memory.limit_in_bytes", func() (string, error) {
				return "18446744073709551615", nil
			})

			limits, err := container.CurrentMemoryLimits()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(limits.LimitInBytes).Should(Equal(uint64(math.MaxUint64)))
		})

		Context("when getting the limit fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeCgroups.WhenGetting("memory", "memory.limit_in_bytes", func() (string, error) {
					return "", disaster
				})
			})

			It("returns the error", func() {
				limits, err := container.CurrentMemoryLimits()
				Ω(err).Should(Equal(disaster))
				Ω(limits).Should(BeZero())
			})
		})
	})

	Describe("Limiting CPU", func() {
		It("sets cpu.shares", func() {
			limits := warden.CPULimits{
				LimitInShares: 512,
			}

			err := container.LimitCPU(limits)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeCgroups.SetValues()).Should(Equal(
				[]fake_cgroups_manager.SetValue{
					{
						Subsystem: "cpu",
						Name:      "cpu.shares",
						Value:     "512",
					},
				},
			))

		})

		Context("when setting cpu.shares fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeCgroups.WhenSetting("cpu", "cpu.shares", func() error {
					return disaster
				})
			})

			It("returns the error", func() {
				err := container.LimitCPU(warden.CPULimits{
					LimitInShares: 512,
				})

				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Getting the current CPU limits", func() {
		It("returns the CPU limits", func() {
			fakeCgroups.WhenGetting("cpu", "cpu.shares", func() (string, error) {
				return "512", nil
			})

			limits, err := container.CurrentCPULimits()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(limits.LimitInShares).Should(Equal(uint64(512)))
		})

		Context("when getting the limit fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeCgroups.WhenGetting("cpu", "cpu.shares", func() (string, error) {
					return "", disaster
				})
			})

			It("returns the error", func() {
				limits, err := container.CurrentCPULimits()
				Ω(err).Should(Equal(disaster))
				Ω(limits).Should(BeZero())
			})
		})
	})

	Describe("Limiting disk", func() {
		limits := warden.DiskLimits{
			BlockSoft: 3,
			BlockHard: 4,

			InodeSoft: 13,
			InodeHard: 14,

			ByteSoft: 23,
			ByteHard: 24,
		}

		It("sets the quota via the quota manager with the uid and limits", func() {
			resultingLimits := warden.DiskLimits{
				BlockHard: 1234567,
			}

			fakeQuotaManager.GetLimitsResult = resultingLimits

			err := container.LimitDisk(limits)
			Ω(err).ShouldNot(HaveOccurred())

			uid := containerResources.UID

			Ω(fakeQuotaManager.Limited).Should(HaveKey(uid))
			Ω(fakeQuotaManager.Limited[uid]).Should(Equal(limits))
		})

		Context("when setting the quota fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeQuotaManager.SetLimitsError = disaster
			})

			It("returns the error", func() {
				err := container.LimitDisk(limits)
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Getting the current disk limits", func() {
		It("returns the disk limits", func() {
			limits := warden.DiskLimits{
				BlockHard: 1234567,
			}

			fakeQuotaManager.GetLimitsResult = limits

			receivedLimits, err := container.CurrentDiskLimits()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(receivedLimits).Should(Equal(limits))
		})

		Context("when getting the limit fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeQuotaManager.GetLimitsError = disaster
			})

			It("returns the error", func() {
				limits, err := container.CurrentDiskLimits()
				Ω(err).Should(Equal(disaster))
				Ω(limits).Should(BeZero())
			})
		})
	})

	Describe("Net in", func() {
		It("executes net.sh in with HOST_PORT and CONTAINER_PORT", func() {
			hostPort, containerPort, err := container.NetIn(123, 456)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/net.sh",
					Args: []string{"in"},
					Env: []string{
						"HOST_PORT=123",
						"CONTAINER_PORT=456",
						"PATH=" + os.Getenv("PATH"),
					},
				},
			))

			Ω(hostPort).Should(Equal(uint32(123)))
			Ω(containerPort).Should(Equal(uint32(456)))
		})

		Context("when a host port is not provided", func() {
			It("acquires one from the port pool", func() {
				hostPort, containerPort, err := container.NetIn(0, 456)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(hostPort).Should(Equal(uint32(1000)))
				Ω(containerPort).Should(Equal(uint32(456)))

				secondHostPort, _, err := container.NetIn(0, 456)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(secondHostPort).ShouldNot(Equal(hostPort))

				Ω(container.Resources().Ports).Should(ContainElement(hostPort))
			})

			Context("and acquiring a port from the pool fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakePortPool.AcquireError = disaster
				})

				It("returns the error", func() {
					_, _, err := container.NetIn(0, 456)
					Ω(err).Should(Equal(disaster))
				})
			})
		})

		Context("when a container port is not provided", func() {
			It("defaults it to the host port", func() {
				hostPort, containerPort, err := container.NetIn(123, 0)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/net.sh",
						Args: []string{"in"},
						Env: []string{
							"HOST_PORT=123",
							"CONTAINER_PORT=123",
							"PATH=" + os.Getenv("PATH"),
						},
					},
				))

				Ω(hostPort).Should(Equal(uint32(123)))
				Ω(containerPort).Should(Equal(uint32(123)))
			})

			Context("and a host port is not provided either", func() {
				It("defaults it to the same acquired port", func() {
					hostPort, containerPort, err := container.NetIn(0, 0)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeRunner).Should(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: containerDir + "/net.sh",
							Args: []string{"in"},
							Env: []string{
								"HOST_PORT=1000",
								"CONTAINER_PORT=1000",
								"PATH=" + os.Getenv("PATH"),
							},
						},
					))

					Ω(hostPort).Should(Equal(uint32(1000)))
					Ω(containerPort).Should(Equal(uint32(1000)))
				})
			})
		})

		Context("when net.sh fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/net.sh",
					}, func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				_, _, err := container.NetIn(123, 456)
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Net out", func() {
		It("executes net.sh out with NETWORK and PORT", func() {
			err := container.NetOut("1.2.3.4/22", 567)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/net.sh",
					Args: []string{"out"},
					Env: []string{
						"NETWORK=1.2.3.4/22",
						"PORT=567",
						"PATH=" + os.Getenv("PATH"),
					},
				},
			))

		})

		Context("when port 0 is given", func() {
			It("executes with PORT as an empty string", func() {
				err := container.NetOut("1.2.3.4/22", 0)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/net.sh",
						Args: []string{"out"},
						Env: []string{
							"NETWORK=1.2.3.4/22",
							"PORT=",
							"PATH=" + os.Getenv("PATH"),
						},
					},
				))

			})

			Context("and a network is not given", func() {
				It("returns an error", func() {
					err := container.NetOut("", 0)
					Ω(err).Should(HaveOccurred())
				})
			})
		})

		Context("when net.sh fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/net.sh",
					}, func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				err := container.NetOut("1.2.3.4/22", 567)
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Info", func() {
		It("returns the container's state", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.State).Should(Equal("born"))
		})

		It("returns the container's events", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Events).Should(Equal([]string{}))
		})

		It("returns the container's properties", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Properties).Should(Equal(container.Properties()))
		})

		It("returns the container's network info", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.HostIP).Should(Equal("10.254.0.1"))
			Ω(info.ContainerIP).Should(Equal("10.254.0.2"))
		})

		It("returns the container's path", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(info.ContainerPath).Should(Equal(containerDir))
		})

		It("returns the container's mapped ports", func() {
			_, _, err := container.NetIn(1234, 5678)
			Ω(err).ShouldNot(HaveOccurred())

			_, _, err = container.NetIn(1235, 5679)
			Ω(err).ShouldNot(HaveOccurred())

			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(info.MappedPorts).Should(Equal([]warden.PortMapping{
				{HostPort: 1234, ContainerPort: 5678},
				{HostPort: 1235, ContainerPort: 5679},
			}))

		})

		Context("with running processes", func() {
			BeforeEach(func() {
				p1 := new(fake_process_tracker.FakeLinuxProcess)
				p1.IDReturns(1)
				p1.WithTTYReturns(true)

				p2 := new(fake_process_tracker.FakeLinuxProcess)
				p2.IDReturns(2)
				p2.WithTTYReturns(false)

				p3 := new(fake_process_tracker.FakeLinuxProcess)
				p3.IDReturns(3)
				p3.WithTTYReturns(true)

				fakeProcessTracker.ActiveProcessesReturns([]process_tracker.LinuxProcess{p1, p2, p3})
			})

			It("returns their process IDs", func() {
				info, err := container.Info()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(info.ProcessIDs).Should(Equal([]uint32{1, 2, 3}))
			})
		})

		Describe("memory info", func() {
			BeforeEach(func() {
				fakeCgroups.WhenGetting("memory", "memory.stat", func() (string, error) {
					return `cache 1
rss 2
mapped_file 3
pgpgin 4
pgpgout 5
swap 6
pgfault 7
pgmajfault 8
inactive_anon 9
active_anon 10
inactive_file 11
active_file 12
unevictable 13
hierarchical_memory_limit 14
hierarchical_memsw_limit 15
total_cache 16
total_rss 17
total_mapped_file 18
total_pgpgin 19
total_pgpgout 20
total_swap 21
total_pgfault 22
total_pgmajfault 23
total_inactive_anon 24
total_active_anon 25
total_inactive_file 26
total_active_file 27
total_unevictable 28
`, nil
				})
			})

			It("is returned in the response", func() {
				info, err := container.Info()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(info.MemoryStat).Should(Equal(warden.ContainerMemoryStat{
					Cache:                   1,
					Rss:                     2,
					MappedFile:              3,
					Pgpgin:                  4,
					Pgpgout:                 5,
					Swap:                    6,
					Pgfault:                 7,
					Pgmajfault:              8,
					InactiveAnon:            9,
					ActiveAnon:              10,
					InactiveFile:            11,
					ActiveFile:              12,
					Unevictable:             13,
					HierarchicalMemoryLimit: 14,
					HierarchicalMemswLimit:  15,
					TotalCache:              16,
					TotalRss:                17,
					TotalMappedFile:         18,
					TotalPgpgin:             19,
					TotalPgpgout:            20,
					TotalSwap:               21,
					TotalPgfault:            22,
					TotalPgmajfault:         23,
					TotalInactiveAnon:       24,
					TotalActiveAnon:         25,
					TotalInactiveFile:       26,
					TotalActiveFile:         27,
					TotalUnevictable:        28,
				}))

			})
		})

		Context("when getting memory.stat fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeCgroups.WhenGetting("memory", "memory.stat", func() (string, error) {
					return "", disaster
				})
			})

			It("returns an error", func() {
				_, err := container.Info()
				Ω(err).Should(Equal(disaster))
			})
		})

		Describe("cpu info", func() {
			BeforeEach(func() {
				fakeCgroups.WhenGetting("cpuacct", "cpuacct.usage", func() (string, error) {
					return `42
`, nil
				})

				fakeCgroups.WhenGetting("cpuacct", "cpuacct.stat", func() (string, error) {
					return `user 1
system 2
`, nil
				})
			})

			It("is returned in the response", func() {
				info, err := container.Info()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(info.CPUStat).Should(Equal(warden.ContainerCPUStat{
					Usage:  42,
					User:   1,
					System: 2,
				}))

			})
		})

		Context("when getting cpuacct/cpuacct.usage fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeCgroups.WhenGetting("cpuacct", "cpuacct.usage", func() (string, error) {
					return "", disaster
				})
			})

			It("returns an error", func() {
				_, err := container.Info()
				Ω(err).Should(Equal(disaster))
			})
		})

		Context("when getting cpuacct/cpuacct.stat fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeCgroups.WhenGetting("cpuacct", "cpuacct.stat", func() (string, error) {
					return "", disaster
				})
			})

			It("returns an error", func() {
				_, err := container.Info()
				Ω(err).Should(Equal(disaster))
			})
		})

		Describe("disk usage info", func() {
			It("is returned in the response", func() {
				fakeQuotaManager.GetUsageResult = warden.ContainerDiskStat{
					BytesUsed:  1,
					InodesUsed: 2,
				}

				info, err := container.Info()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(info.DiskStat).Should(Equal(warden.ContainerDiskStat{
					BytesUsed:  1,
					InodesUsed: 2,
				}))

			})

			Context("when getting the disk usage fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeQuotaManager.GetUsageError = disaster
				})

				It("returns the error", func() {
					_, err := container.Info()
					Ω(err).Should(Equal(disaster))
				})
			})
		})

		Describe("bandwidth info", func() {
			It("is returned in the response", func() {
				fakeBandwidthManager.GetLimitsResult = warden.ContainerBandwidthStat{
					InRate:   1,
					InBurst:  2,
					OutRate:  3,
					OutBurst: 4,
				}

				info, err := container.Info()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(info.BandwidthStat).Should(Equal(warden.ContainerBandwidthStat{
					InRate:   1,
					InBurst:  2,
					OutRate:  3,
					OutBurst: 4,
				}))

			})

			Context("when getting the bandwidth usage fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeBandwidthManager.GetLimitsError = disaster
				})

				It("returns the error", func() {
					_, err := container.Info()
					Ω(err).Should(Equal(disaster))
				})
			})
		})
	})
})

func uint64ptr(n uint64) *uint64 {
	return &n
}
