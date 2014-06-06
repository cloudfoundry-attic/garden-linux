package linux_backend_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"math"
	"net"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/bandwidth_manager/fake_bandwidth_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/network_pool"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/port_pool/fake_port_pool"
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

var _ = Describe("Linux containers", func() {
	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeCgroups = fake_cgroups_manager.New("/cgroups", "some-id")

		fakeQuotaManager = fake_quota_manager.New()
		fakeBandwidthManager = fake_bandwidth_manager.New()
		fakeProcessTracker = new(fake_process_tracker.FakeProcessTracker)

		_, ipNet, err := net.ParseCIDR("10.254.0.0/24")
		Expect(err).ToNot(HaveOccurred())

		fakePortPool = fake_port_pool.New(1000)

		networkPool := network_pool.New(ipNet)

		network, err := networkPool.Acquire()
		Expect(err).ToNot(HaveOccurred())

		containerResources = linux_backend.NewResources(
			1234,
			network,
			[]uint32{},
		)

		container = linux_backend.NewLinuxContainer(
			"some-id",
			"some-handle",
			"/depot/some-id",
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
		)
	})

	outChunk := warden.ProcessStream{
		Source: warden.ProcessStreamSourceStdout,
		Data:   []byte("hi out\n"),
	}

	errChunk := warden.ProcessStream{
		Source: warden.ProcessStreamSourceStderr,
		Data:   []byte("hi err\n"),
	}

	exit42 := uint32(42)
	exitChunk := warden.ProcessStream{
		ExitStatus: &exit42,
	}

	Describe("Snapshotting", func() {
		memoryLimits := warden.MemoryLimits{
			LimitInBytes: 1,
		}

		diskLimits := warden.DiskLimits{
			BlockLimit: 1,
			Block:      2,
			BlockSoft:  3,
			BlockHard:  4,

			InodeLimit: 11,
			Inode:      12,
			InodeSoft:  13,
			InodeHard:  14,

			ByteLimit: 21,
			Byte:      22,
			ByteSoft:  23,
			ByteHard:  24,
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
			Expect(err).ToNot(HaveOccurred())

			_, _, err = container.NetIn(1, 2)
			Expect(err).ToNot(HaveOccurred())

			_, _, err = container.NetIn(3, 4)
			Expect(err).ToNot(HaveOccurred())

			err = container.NetOut("network-a", 1)
			Expect(err).ToNot(HaveOccurred())

			err = container.NetOut("network-b", 2)
			Expect(err).ToNot(HaveOccurred())

			fakeProcessTracker.ActiveProcessIDsReturns([]uint32{1, 2, 3})
		})

		It("writes a JSON ContainerSnapshot", func() {
			out := new(bytes.Buffer)

			err := container.Snapshot(out)
			Expect(err).ToNot(HaveOccurred())

			var snapshot linux_backend.ContainerSnapshot

			err = json.NewDecoder(out).Decode(&snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(snapshot.ID).To(Equal("some-id"))
			Expect(snapshot.Handle).To(Equal("some-handle"))

			Expect(snapshot.GraceTime).To(Equal(1 * time.Second))

			Expect(snapshot.State).To(Equal("active"))

			Expect(snapshot.Resources).To(Equal(
				linux_backend.ResourcesSnapshot{
					UID:     containerResources.UID,
					Network: containerResources.Network,
					Ports:   containerResources.Ports,
				},
			))

			Expect(snapshot.NetIns).To(Equal(
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

			Expect(snapshot.NetOuts).To(Equal(
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

			Expect(snapshot.Processes).To(ContainElement(
				linux_backend.ProcessSnapshot{
					ID: 1,
				},
			))

			Expect(snapshot.Processes).To(ContainElement(
				linux_backend.ProcessSnapshot{
					ID: 2,
				},
			))

			Expect(snapshot.Processes).To(ContainElement(
				linux_backend.ProcessSnapshot{
					ID: 3,
				},
			))

			Expect(snapshot.Properties).To(Equal(warden.Properties(map[string]string{
				"property-name": "property-value",
			})))
		})

		Context("with limits set", func() {
			BeforeEach(func() {
				err := container.LimitMemory(memoryLimits)
				Expect(err).ToNot(HaveOccurred())

				// oom exits immediately since it's faked out; should see event,
				// and it should show up in the snapshot
				Eventually(container.Events).Should(ContainElement("out of memory"))

				err = container.LimitDisk(diskLimits)
				Expect(err).ToNot(HaveOccurred())

				err = container.LimitBandwidth(bandwidthLimits)
				Expect(err).ToNot(HaveOccurred())

				err = container.LimitCPU(cpuLimits)
				Expect(err).ToNot(HaveOccurred())
			})

			It("saves them", func() {
				out := new(bytes.Buffer)

				err := container.Snapshot(out)
				Expect(err).ToNot(HaveOccurred())

				var snapshot linux_backend.ContainerSnapshot

				err = json.NewDecoder(out).Decode(&snapshot)
				Expect(err).ToNot(HaveOccurred())

				Expect(snapshot.State).To(Equal("stopped"))
				Expect(snapshot.Events).To(Equal([]string{"out of memory"}))

				Expect(snapshot.Limits).To(Equal(
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
				Expect(err).ToNot(HaveOccurred())

				var snapshot linux_backend.ContainerSnapshot

				err = json.NewDecoder(out).Decode(&snapshot)
				Expect(err).ToNot(HaveOccurred())

				Expect(snapshot.Limits).To(Equal(
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
			Expect(err).ToNot(HaveOccurred())

			Expect(container.State()).To(Equal(linux_backend.State("active")))
			Expect(container.Events()).To(Equal([]string{
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
						ID: 0,
					},
					{
						ID: 1,
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())

			Ω(fakeProcessTracker.RestoreArgsForCall(0)).Should(Equal(uint32(0)))
			Ω(fakeProcessTracker.RestoreArgsForCall(1)).Should(Equal(uint32(1)))
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
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/net.sh",
					Args: []string{"setup"},
				},
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/net.sh",
					Args: []string{"in"},
				},
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/net.sh",
					Args: []string{"in"},
				},
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/net.sh",
					Args: []string{"out"},
				},
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/net.sh",
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
							Path: "/depot/some-id/net.sh",
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
					Expect(err).To(Equal(disaster))
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
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeCgroups.SetValues()).To(ContainElement(
				fake_cgroups_manager.SetValue{
					Subsystem: "memory",
					Name:      "memory.limit_in_bytes",
					Value:     "1024",
				},
			))

			Expect(fakeCgroups.SetValues()).To(ContainElement(
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
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeCgroups.SetValues()).To(BeEmpty())
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
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Starting", func() {
		It("executes the container's start.sh with the correct environment", func() {
			err := container.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/start.sh",
					Env: []string{
						"id=some-id",
						"container_iface_mtu=1500",
						"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
					},
				},
			))
		})

		It("changes the container's state to active", func() {
			Expect(container.State()).To(Equal(linux_backend.StateBorn))

			err := container.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(container.State()).To(Equal(linux_backend.StateActive))
		})

		Context("when start.sh fails", func() {
			nastyError := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/start.sh",
					}, func(*exec.Cmd) error {
						return nastyError
					},
				)
			})

			It("returns the error", func() {
				err := container.Start()
				Expect(err).To(Equal(nastyError))
			})

			It("does not change the container's state", func() {
				Expect(container.State()).To(Equal(linux_backend.StateBorn))

				err := container.Start()
				Expect(err).To(HaveOccurred())

				Expect(container.State()).To(Equal(linux_backend.StateBorn))
			})
		})
	})

	Describe("Stopping", func() {
		It("executes the container's stop.sh", func() {
			err := container.Stop(false)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/stop.sh",
				},
			))
		})

		It("sets the container's state to stopped", func() {
			Expect(container.State()).To(Equal(linux_backend.StateBorn))

			err := container.Stop(false)
			Expect(err).ToNot(HaveOccurred())

			Expect(container.State()).To(Equal(linux_backend.StateStopped))

		})

		Context("when kill is true", func() {
			It("executes stop.sh with -w 0", func() {
				err := container.Stop(true)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/stop.sh",
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
						Path: "/depot/some-id/stop.sh",
					}, func(*exec.Cmd) error {
						return nastyError
					},
				)
			})

			It("returns the error", func() {
				err := container.Stop(false)
				Expect(err).To(Equal(nastyError))
			})

			It("does not change the container's state", func() {
				Expect(container.State()).To(Equal(linux_backend.StateBorn))

				err := container.Stop(false)
				Expect(err).To(HaveOccurred())

				Expect(container.State()).To(Equal(linux_backend.StateBorn))
			})
		})

		Context("when the container has an oom notifier running", func() {
			BeforeEach(func() {
				err := container.LimitMemory(warden.MemoryLimits{
					LimitInBytes: 42,
				})

				Expect(err).ToNot(HaveOccurred())
			})

			It("stops it", func() {
				err := container.Stop(false)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveKilled(fake_command_runner.CommandSpec{
					Path: "/depot/some-id/bin/oom",
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

				Expect(err).ToNot(HaveOccurred())
			})

			It("stops it", func() {
				container.Cleanup()

				Expect(fakeRunner).To(HaveKilled(fake_command_runner.CommandSpec{
					Path: "/depot/some-id/bin/oom",
				}))
			})
		})
	})

	Describe("Streaming data in", func() {
		var source io.Reader

		BeforeEach(func() {
			source = strings.NewReader("the-tar-content")
		})

		It("streams the input to tar xf in the container", func(done Done) {
			fakeRunner.WhenRunning(
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/bin/wsh",
					Args: []string{
						"--socket", "/depot/some-id/run/wshd.sock",
						"--user", "vcap",
						"bash", "-c", `mkdir -p /some/directory/dst && tar xf - -C /some/directory/dst`,
					},
				},
				func(cmd *exec.Cmd) error {
					go func() {
						defer GinkgoRecover()

						bytes, err := ioutil.ReadAll(cmd.Stdin)
						Expect(err).ToNot(HaveOccurred())

						Expect(string(bytes)).To(Equal("the-tar-content"))

						close(done)
					}()

					return nil
				},
			)

			writer, err := container.StreamIn("/some/directory/dst")
			Expect(err).ToNot(HaveOccurred())

			writer.Write([]byte("the-tar-content"))
			writer.Close()
		})

		Context("and closing the stream", func() {
			It("closes the command's input and waits for tar to complete", func() {
				waited := make(chan struct{})

				fakeRunner.WhenWaitingFor(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/bin/wsh",
					},
					func(cmd *exec.Cmd) error {
						_, err := cmd.Stdin.Read(nil)
						Ω(err).Should(Equal(io.EOF))

						close(waited)
						return nil
					},
				)

				writer, err := container.StreamIn("/some/directory/dst")
				Expect(err).ToNot(HaveOccurred())

				err = writer.Close()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(waited).Should(BeClosed())
			})

			Context("when tar fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRunner.WhenWaitingFor(
						fake_command_runner.CommandSpec{
							Path: "/depot/some-id/bin/wsh",
						},
						func(cmd *exec.Cmd) error {
							return disaster
						},
					)
				})

				It("returns the error", func() {
					writer, err := container.StreamIn("/some/directory/dst")
					Expect(err).ToNot(HaveOccurred())

					err = writer.Close()
					Ω(err).Should(Equal(disaster))
				})
			})
		})

		Context("when executing the command fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/bin/wsh",
					}, func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				_, err := container.StreamIn("/some/dst")
				Expect(err).To(Equal(disaster))
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
					Path: "/depot/some-id/bin/wsh",
					Args: []string{
						"--socket", "/depot/some-id/run/wshd.sock",
						"--user", "vcap",
						"tar", "cf", "-", "-C", "/some/directory", "dst",
					},
				},
				func(cmd *exec.Cmd) error {
					go cmd.Stdout.Write([]byte("the-compressed-content"))
					return nil
				},
			)

			reader, err := container.StreamOut("/some/directory/dst")
			Expect(err).ToNot(HaveOccurred())

			bytes, err := ioutil.ReadAll(reader)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(bytes)).To(Equal("the-compressed-content"))
		})

		Context("when there's a trailing slash", func() {
			It("compresses the directory's contents", func() {
				_, err := container.StreamOut("/some/directory/dst/")
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveBackgrounded(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/bin/wsh",
						Args: []string{
							"--socket", "/depot/some-id/run/wshd.sock",
							"--user", "vcap",
							"tar", "cf", "-", "-C", "/some/directory/dst/", ".",
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
						Path: "/depot/some-id/bin/wsh",
					}, func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				_, err := container.StreamOut("/some/dst")
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Running", func() {
		It("runs the /bin/bash via wsh with the given script as the input, and rlimits in env", func() {
			_, _, err := container.Run(warden.ProcessSpec{
				Script: "/some/script",
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
			})

			Expect(err).ToNot(HaveOccurred())

			ranCmd := fakeProcessTracker.RunArgsForCall(0)
			Ω(ranCmd.Path).Should(Equal("/depot/some-id/bin/wsh"))

			Ω(ranCmd.Args).Should(Equal([]string{
				"--socket", "/depot/some-id/run/wshd.sock",
				"--user", "vcap",
				"/bin/bash",
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

			stdin, err := ioutil.ReadAll(ranCmd.Stdin)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(stdin)).Should(Equal("/some/script"))
		})

		It("runs the script with escaped environment variables", func() {
			_, _, err := container.Run(warden.ProcessSpec{
				Script: "/some/script",
				EnvironmentVariables: []warden.EnvironmentVariable{
					warden.EnvironmentVariable{Key: "ESCAPED", Value: "kurt \"russell\""},
					warden.EnvironmentVariable{Key: "INTERPOLATED", Value: "snake $PLISSKEN"},
					warden.EnvironmentVariable{Key: "UNESCAPED", Value: "isaac\nhayes"},
				},
			})

			Expect(err).ToNot(HaveOccurred())

			ranCmd := fakeProcessTracker.RunArgsForCall(0)

			stdin, err := ioutil.ReadAll(ranCmd.Stdin)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(stdin)).Should(Equal(`export ESCAPED="kurt \"russell\""
export INTERPOLATED="snake $PLISSKEN"
export UNESCAPED="isaac
hayes"
/some/script`))
		})

		Describe("streaming", func() {
			BeforeEach(func() {
				preloadedChannel := make(chan warden.ProcessStream, 3)

				preloadedChannel <- outChunk
				preloadedChannel <- errChunk
				preloadedChannel <- exitChunk

				fakeProcessTracker.RunReturns(1, preloadedChannel, nil)
			})

			It("streams stderr and stdout and exit status", func() {
				_, runningStreamChannel, err := container.Run(warden.ProcessSpec{
					Script: "/some/script",
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(runningStreamChannel).Should(Receive(Equal(outChunk)))
				Eventually(runningStreamChannel).Should(Receive(Equal(errChunk)))
				Eventually(runningStreamChannel).Should(Receive(Equal(exitChunk)))
			})
		})

		Context("when not all rlimits are set", func() {
			It("only sets the given rlimits", func() {
				_, _, err := container.Run(warden.ProcessSpec{
					Script: "/some/script",
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
				})

				Expect(err).ToNot(HaveOccurred())

				ranCmd := fakeProcessTracker.RunArgsForCall(0)
				Ω(ranCmd.Path).Should(Equal("/depot/some-id/bin/wsh"))

				Ω(ranCmd.Args).Should(Equal([]string{
					"--socket", "/depot/some-id/run/wshd.sock",
					"--user", "vcap",
					"/bin/bash",
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
		})

		It("returns a the process ID determined by the process tracker", func() {
			fakeProcessTracker.RunReturns(1, nil, nil)

			processID1, _, err := container.Run(warden.ProcessSpec{
				Script: "/some/script",
			})
			Ω(err).ToNot(HaveOccurred())
			Ω(processID1).Should(Equal(uint32(1)))

			fakeProcessTracker.RunReturns(2, nil, nil)

			processID2, _, err := container.Run(warden.ProcessSpec{
				Script: "/some/script",
			})
			Ω(err).ToNot(HaveOccurred())
			Ω(processID2).Should(Equal(uint32(2)))
		})

		Context("with 'privileged' true", func() {
			It("runs with --user root", func() {
				_, _, err := container.Run(warden.ProcessSpec{
					Script:     "/some/script",
					Privileged: true,
				})

				Ω(err).ToNot(HaveOccurred())

				ranCmd := fakeProcessTracker.RunArgsForCall(0)
				Ω(ranCmd.Path).Should(Equal("/depot/some-id/bin/wsh"))

				Ω(ranCmd.Args).Should(Equal([]string{
					"--socket", "/depot/some-id/run/wshd.sock",
					"--user", "root",
					"/bin/bash",
				}))
			})
		})

		Context("when spawning fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeProcessTracker.RunReturns(0, nil, disaster)
			})

			It("returns the error", func() {
				_, _, err := container.Run(warden.ProcessSpec{
					Script:     "/some/script",
					Privileged: true,
				})

				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Attaching", func() {
		Context("to a started process", func() {
			BeforeEach(func() {
				preloadedChannel := make(chan warden.ProcessStream, 3)

				preloadedChannel <- outChunk
				preloadedChannel <- errChunk
				preloadedChannel <- exitChunk

				fakeProcessTracker.AttachReturns(preloadedChannel, nil)
			})

			It("streams stderr and stdout and exit status", func(done Done) {
				attachStreamChannel, err := container.Attach(1)
				Expect(err).ToNot(HaveOccurred())

				Ω(fakeProcessTracker.AttachArgsForCall(0)).Should(Equal(uint32(1)))

				attachChunk := <-attachStreamChannel
				Expect(attachChunk.Source).To(Equal(warden.ProcessStreamSourceStdout))
				Expect(string(attachChunk.Data)).To(Equal("hi out\n"))
				Expect(attachChunk.ExitStatus).To(BeNil())

				attachChunk = <-attachStreamChannel
				Expect(attachChunk.Source).To(Equal(warden.ProcessStreamSourceStderr))
				Expect(string(attachChunk.Data)).To(Equal("hi err\n"))
				Expect(attachChunk.ExitStatus).To(BeNil())

				attachChunk = <-attachStreamChannel
				Expect(attachChunk.Source).To(BeZero())
				Expect(string(attachChunk.Data)).To(Equal(""))
				Expect(attachChunk.ExitStatus).ToNot(BeNil())
				Expect(*attachChunk.ExitStatus).To(Equal(uint32(42)))

				close(done)
			}, 5.0)
		})

		Context("when attaching fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeProcessTracker.AttachReturns(nil, disaster)
			})

			It("returns the error", func() {
				_, err := container.Attach(42)
				Expect(err).To(Equal(disaster))
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
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeBandwidthManager.EnforcedLimits).To(ContainElement(limits))
		})

		Context("when setting the limit fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeBandwidthManager.SetLimitsError = disaster
			})

			It("returns the error", func() {
				err := container.LimitBandwidth(limits)
				Expect(err).To(Equal(disaster))
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
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedLimits).To(BeZero())
		})

		Context("when limits are set", func() {
			It("returns them", func() {
				err := container.LimitBandwidth(limits)
				Expect(err).ToNot(HaveOccurred())

				receivedLimits, err := container.CurrentBandwidthLimits()
				Expect(err).ToNot(HaveOccurred())
				Expect(receivedLimits).To(Equal(limits))
			})

			Context("when limits fail to be set", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeBandwidthManager.SetLimitsError = disaster
				})

				It("does not update the current limits", func() {
					err := container.LimitBandwidth(limits)
					Expect(err).To(Equal(disaster))

					receivedLimits, err := container.CurrentBandwidthLimits()
					Expect(err).ToNot(HaveOccurred())
					Expect(receivedLimits).To(BeZero())
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
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveStartedExecuting(
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/bin/oom",
					Args: []string{"/cgroups/memory/instance-some-id"},
				},
			))
		})

		It("sets memory.limit_in_bytes and then memory.memsw.limit_in_bytes", func() {
			limits := warden.MemoryLimits{
				LimitInBytes: 102400,
			}

			err := container.LimitMemory(limits)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeCgroups.SetValues()).To(Equal(
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
					Path: "/depot/some-id/bin/oom",
				}, func(*exec.Cmd) error {
					started++
					return nil
				})

				limits := warden.MemoryLimits{
					LimitInBytes: 102400,
				}

				err := container.LimitMemory(limits)
				Expect(err).ToNot(HaveOccurred())

				err = container.LimitMemory(limits)
				Expect(err).ToNot(HaveOccurred())

				Expect(started).To(Equal(1))
			})
		})

		Context("when the oom notifier exits 0", func() {
			BeforeEach(func() {
				fakeRunner.WhenWaitingFor(fake_command_runner.CommandSpec{
					Path: "/depot/some-id/bin/oom",
				}, func(cmd *exec.Cmd) error {
					return nil
				})
			})

			It("stops the container", func() {
				limits := warden.MemoryLimits{
					LimitInBytes: 102400,
				}

				err := container.LimitMemory(limits)
				Expect(err).ToNot(HaveOccurred())

				Eventually(fakeRunner).Should(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/stop.sh",
					},
				))
			})

			It("registers an 'out of memory' event", func() {
				limits := warden.MemoryLimits{
					LimitInBytes: 102400,
				}

				err := container.LimitMemory(limits)
				Expect(err).ToNot(HaveOccurred())

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

				Expect(err).ToNot(HaveOccurred())
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

				Expect(err).ToNot(HaveOccurred())
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

				Expect(err).To(Equal(disaster))
			})
		})

		Context("when starting the oom notifier fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(fake_command_runner.CommandSpec{
					Path: "/depot/some-id/bin/oom",
				}, func(cmd *exec.Cmd) error {
					return disaster
				})
			})

			It("returns the error", func() {
				err := container.LimitMemory(warden.MemoryLimits{
					LimitInBytes: 102400,
				})

				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Getting the current memory limit", func() {
		It("returns the limited memory", func() {
			fakeCgroups.WhenGetting("memory", "memory.limit_in_bytes", func() (string, error) {
				return "18446744073709551615", nil
			})

			limits, err := container.CurrentMemoryLimits()
			Expect(err).ToNot(HaveOccurred())
			Expect(limits.LimitInBytes).To(Equal(uint64(math.MaxUint64)))
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
				Expect(err).To(Equal(disaster))
				Expect(limits).To(BeZero())
			})
		})
	})

	Describe("Limiting CPU", func() {
		It("sets cpu.shares", func() {
			limits := warden.CPULimits{
				LimitInShares: 512,
			}

			err := container.LimitCPU(limits)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeCgroups.SetValues()).To(Equal(
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

				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Getting the current CPU limits", func() {
		It("returns the CPU limits", func() {
			fakeCgroups.WhenGetting("cpu", "cpu.shares", func() (string, error) {
				return "512", nil
			})

			limits, err := container.CurrentCPULimits()
			Expect(err).ToNot(HaveOccurred())
			Expect(limits.LimitInShares).To(Equal(uint64(512)))
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
				Expect(err).To(Equal(disaster))
				Expect(limits).To(BeZero())
			})
		})
	})

	Describe("Limiting disk", func() {
		limits := warden.DiskLimits{
			BlockLimit: 1,
			Block:      2,
			BlockSoft:  3,
			BlockHard:  4,

			InodeLimit: 11,
			Inode:      12,
			InodeSoft:  13,
			InodeHard:  14,

			ByteLimit: 21,
			Byte:      22,
			ByteSoft:  23,
			ByteHard:  24,
		}

		It("sets the quota via the quota manager with the uid and limits", func() {
			resultingLimits := warden.DiskLimits{
				Block: 1234567,
			}

			fakeQuotaManager.GetLimitsResult = resultingLimits

			err := container.LimitDisk(limits)
			Expect(err).ToNot(HaveOccurred())

			uid := containerResources.UID

			Expect(fakeQuotaManager.Limited).To(HaveKey(uid))
			Expect(fakeQuotaManager.Limited[uid]).To(Equal(limits))
		})

		Context("when setting the quota fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeQuotaManager.SetLimitsError = disaster
			})

			It("returns the error", func() {
				err := container.LimitDisk(limits)
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Getting the current disk limits", func() {
		It("returns the disk limits", func() {
			limits := warden.DiskLimits{
				Block: 1234567,
			}

			fakeQuotaManager.GetLimitsResult = limits

			receivedLimits, err := container.CurrentDiskLimits()
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedLimits).To(Equal(limits))
		})

		Context("when getting the limit fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeQuotaManager.GetLimitsError = disaster
			})

			It("returns the error", func() {
				limits, err := container.CurrentDiskLimits()
				Expect(err).To(Equal(disaster))
				Expect(limits).To(BeZero())
			})
		})
	})

	Describe("Net in", func() {
		It("executes net.sh in with HOST_PORT and CONTAINER_PORT", func() {
			hostPort, containerPort, err := container.NetIn(123, 456)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/net.sh",
					Args: []string{"in"},
					Env: []string{
						"HOST_PORT=123",
						"CONTAINER_PORT=456",
					},
				},
			))

			Expect(hostPort).To(Equal(uint32(123)))
			Expect(containerPort).To(Equal(uint32(456)))
		})

		Context("when a host port is not provided", func() {
			It("acquires one from the port pool", func() {
				hostPort, containerPort, err := container.NetIn(0, 456)
				Expect(err).ToNot(HaveOccurred())

				Expect(hostPort).To(Equal(uint32(1000)))
				Expect(containerPort).To(Equal(uint32(456)))

				secondHostPort, _, err := container.NetIn(0, 456)
				Expect(err).ToNot(HaveOccurred())

				Expect(secondHostPort).ToNot(Equal(hostPort))

				Expect(container.Resources().Ports).To(ContainElement(hostPort))
			})

			Context("and acquiring a port from the pool fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakePortPool.AcquireError = disaster
				})

				It("returns the error", func() {
					_, _, err := container.NetIn(0, 456)
					Expect(err).To(Equal(disaster))
				})
			})
		})

		Context("when a container port is not provided", func() {
			It("defaults it to the host port", func() {
				hostPort, containerPort, err := container.NetIn(123, 0)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/net.sh",
						Args: []string{"in"},
						Env: []string{
							"HOST_PORT=123",
							"CONTAINER_PORT=123",
						},
					},
				))

				Expect(hostPort).To(Equal(uint32(123)))
				Expect(containerPort).To(Equal(uint32(123)))
			})

			Context("and a host port is not provided either", func() {
				It("defaults it to the same acquired port", func() {
					hostPort, containerPort, err := container.NetIn(0, 0)
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeRunner).To(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: "/depot/some-id/net.sh",
							Args: []string{"in"},
							Env: []string{
								"HOST_PORT=1000",
								"CONTAINER_PORT=1000",
							},
						},
					))

					Expect(hostPort).To(Equal(uint32(1000)))
					Expect(containerPort).To(Equal(uint32(1000)))
				})
			})
		})

		Context("when net.sh fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/net.sh",
					}, func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				_, _, err := container.NetIn(123, 456)
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Net out", func() {
		It("executes net.sh out with NETWORK and PORT", func() {
			err := container.NetOut("1.2.3.4/22", 567)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/depot/some-id/net.sh",
					Args: []string{"out"},
					Env: []string{
						"NETWORK=1.2.3.4/22",
						"PORT=567",
					},
				},
			))
		})

		Context("when port 0 is given", func() {
			It("executes with PORT as an empty string", func() {
				err := container.NetOut("1.2.3.4/22", 0)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/net.sh",
						Args: []string{"out"},
						Env: []string{
							"NETWORK=1.2.3.4/22",
							"PORT=",
						},
					},
				))
			})

			Context("and a network is not given", func() {
				It("returns an error", func() {
					err := container.NetOut("", 0)
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context("when net.sh fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/depot/some-id/net.sh",
					}, func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				err := container.NetOut("1.2.3.4/22", 567)
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Info", func() {
		It("returns the container's state", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.State).To(Equal("born"))
		})

		It("returns the container's events", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.Events).To(Equal([]string{}))
		})

		It("returns the container's properties", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.Properties).To(Equal(container.Properties()))
		})

		It("returns the container's network info", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.HostIP).To(Equal("10.254.0.1"))
			Expect(info.ContainerIP).To(Equal("10.254.0.2"))
		})

		It("returns the container's path", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())
			Expect(info.ContainerPath).To(Equal("/depot/some-id"))
		})

		It("returns the container's mapped ports", func() {
			_, _, err := container.NetIn(1234, 5678)
			Expect(err).ToNot(HaveOccurred())

			_, _, err = container.NetIn(1235, 5679)
			Expect(err).ToNot(HaveOccurred())

			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())
			Expect(info.MappedPorts).To(Equal([]warden.PortMapping{
				{HostPort: 1234, ContainerPort: 5678},
				{HostPort: 1235, ContainerPort: 5679},
			}))
		})

		Context("with running processes", func() {
			BeforeEach(func() {
				fakeProcessTracker.ActiveProcessIDsReturns([]uint32{1, 2, 3})
			})

			It("returns their process IDs", func() {
				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())
				Expect(info.ProcessIDs).To(Equal([]uint32{1, 2, 3}))
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
				Expect(err).ToNot(HaveOccurred())
				Expect(info.MemoryStat).To(Equal(warden.ContainerMemoryStat{
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
				Expect(err).To(Equal(disaster))
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
				Expect(err).ToNot(HaveOccurred())
				Expect(info.CPUStat).To(Equal(warden.ContainerCPUStat{
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
				Expect(err).To(Equal(disaster))
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
				Expect(err).To(Equal(disaster))
			})
		})

		Describe("disk usage info", func() {
			It("is returned in the response", func() {
				fakeQuotaManager.GetUsageResult = warden.ContainerDiskStat{
					BytesUsed:  1,
					InodesUsed: 2,
				}

				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())

				Expect(info.DiskStat).To(Equal(warden.ContainerDiskStat{
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
					Expect(err).To(Equal(disaster))
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
				Expect(err).ToNot(HaveOccurred())

				Expect(info.BandwidthStat).To(Equal(warden.ContainerBandwidthStat{
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
					Expect(err).To(Equal(disaster))
				})
			})
		})
	})
})

func uint64ptr(n uint64) *uint64 {
	return &n
}
