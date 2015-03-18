package linux_container_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var fakeCgroups *fake_cgroups_manager.FakeCgroupsManager
var fakeQuotaManager *fake_quota_manager.FakeQuotaManager
var fakeBandwidthManager *fake_bandwidth_manager.FakeBandwidthManager
var fakeRunner *fake_command_runner.FakeCommandRunner
var containerResources *linux_backend.Resources
var container *linux_container.LinuxContainer
var fakePortPool *fake_port_pool.FakePortPool
var fakeProcessTracker *fake_process_tracker.FakeProcessTracker
var fakeFilter *networkFakes.FakeFilter
var containerDir string
var containerProps map[string]string
var mtu uint32

var oldLang string

var _ = Describe("Linux containers", func() {
	netOutRule1 := garden.NetOutRule{
		Protocol: garden.ProtocolUDP,
		Networks: []garden.IPRange{garden.IPRangeFromIP(net.ParseIP("1.2.3.4"))},
		Ports:    []garden.PortRange{{Start: 12, End: 24}},
		ICMPs:    &garden.ICMPControl{Type: 3, Code: garden.ICMPControlCode(12)},
		Log:      true,
	}

	netOutRule2 := garden.NetOutRule{
		Protocol: garden.ProtocolTCP,
		Networks: []garden.IPRange{garden.IPRangeFromIP(net.ParseIP("1.2.5.4"))},
		Ports:    []garden.PortRange{{Start: 13, End: 34}},
		ICMPs:    &garden.ICMPControl{Type: 3, Code: garden.ICMPControlCode(5)},
		Log:      false,
	}

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeCgroups = fake_cgroups_manager.New("/cgroups", "some-id")

		fakeQuotaManager = fake_quota_manager.New()
		fakeBandwidthManager = fake_bandwidth_manager.New()
		fakeProcessTracker = new(fake_process_tracker.FakeProcessTracker)
		fakeFilter = new(networkFakes.FakeFilter)

		fakePortPool = fake_port_pool.New(1000)

		var err error
		containerDir, err = ioutil.TempDir("", "depot")
		Ω(err).ShouldNot(HaveOccurred())

		err = os.Mkdir(filepath.Join(containerDir, "run"), 0755)
		Ω(err).ShouldNot(HaveOccurred())
		err = ioutil.WriteFile(filepath.Join(containerDir, "run", "wshd.pid"), []byte("12345\n"), 0644)
		Ω(err).ShouldNot(HaveOccurred())

		_, subnet, err := net.ParseCIDR("2.3.4.0/30")
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

		mtu = 1500

		containerProps = map[string]string{
			"property-name": "property-value",
		}

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
			containerProps,
			1*time.Second,
			containerResources,
			fakePortPool,
			fakeRunner,
			fakeCgroups,
			fakeQuotaManager,
			fakeBandwidthManager,
			fakeProcessTracker,
			process.Env{"env1": "env1Value", "env2": "env2Value"},
			fakeFilter,
		)
	})

	It("sets the container ID", func() {
		Ω(container.ID()).Should(Equal("some-id"))
	})

	It("sets the container handle", func() {
		Ω(container.Handle()).Should(Equal("some-handle"))
	})

	It("sets the container grace time", func() {
		Ω(container.GraceTime()).Should(Equal(1 * time.Second))
	})

	Describe("Snapshotting", func() {
		memoryLimits := garden.MemoryLimits{
			LimitInBytes: 1,
		}

		diskLimits := garden.DiskLimits{
			BlockSoft: 3,
			BlockHard: 4,

			InodeSoft: 13,
			InodeHard: 14,

			ByteSoft: 23,
			ByteHard: 24,
		}

		bandwidthLimits := garden.BandwidthLimits{
			RateInBytesPerSecond:      1,
			BurstRateInBytesPerSecond: 2,
		}

		cpuLimits := garden.CPULimits{
			LimitInShares: 1,
		}

		JustBeforeEach(func() {
			var err error

			err = container.Start()
			Ω(err).ShouldNot(HaveOccurred())

			_, _, err = container.NetIn(1, 2)
			Ω(err).ShouldNot(HaveOccurred())

			_, _, err = container.NetIn(3, 4)
			Ω(err).ShouldNot(HaveOccurred())

			container.NetOut(netOutRule1)
			container.NetOut(netOutRule2)

			p1 := new(wfakes.FakeProcess)
			p1.IDReturns(1)

			p2 := new(wfakes.FakeProcess)
			p2.IDReturns(2)

			p3 := new(wfakes.FakeProcess)
			p3.IDReturns(3)

			fakeProcessTracker.ActiveProcessesReturns([]garden.Process{p1, p2, p3})
		})

		It("writes a JSON ContainerSnapshot", func() {
			out := new(bytes.Buffer)

			err := container.Snapshot(out)
			Ω(err).ShouldNot(HaveOccurred())

			var snapshot linux_container.ContainerSnapshot

			err = json.NewDecoder(out).Decode(&snapshot)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(snapshot.ID).Should(Equal("some-id"))
			Ω(snapshot.Handle).Should(Equal("some-handle"))

			Ω(snapshot.GraceTime).Should(Equal(1 * time.Second))

			Ω(snapshot.State).Should(Equal("active"))

			_, subnet, err := net.ParseCIDR("2.3.4.0/30")
			Ω(snapshot.Resources).Should(Equal(
				linux_container.ResourcesSnapshot{
					UserUID: containerResources.UserUID,
					RootUID: containerResources.RootUID,
					Network: &linux_backend.Network{
						IP:     net.ParseIP("1.2.3.4"),
						Subnet: subnet,
					},
					Bridge: "some-bridge",
					Ports:  containerResources.Ports,
				},
			))

			Ω(snapshot.NetIns).Should(Equal(
				[]linux_container.NetInSpec{
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

			Ω(snapshot.NetOuts).Should(Equal([]garden.NetOutRule{
				netOutRule1, netOutRule2,
			}))

			Ω(snapshot.Processes).Should(ContainElement(
				linux_container.ProcessSnapshot{
					ID: 1,
				},
			))

			Ω(snapshot.Processes).Should(ContainElement(
				linux_container.ProcessSnapshot{
					ID: 2,
				},
			))

			Ω(snapshot.Processes).Should(ContainElement(
				linux_container.ProcessSnapshot{
					ID: 3,
				},
			))

			Ω(snapshot.Properties).Should(Equal(garden.Properties(map[string]string{
				"property-name": "property-value",
			})))

			Ω(snapshot.EnvVars).Should(Equal([]string{"env1=env1Value", "env2=env2Value"}))
		})

		Context("with limits set", func() {
			JustBeforeEach(func() {
				err := container.LimitMemory(memoryLimits)
				Ω(err).ShouldNot(HaveOccurred())

				// oom exits immediately since it's faked out; should see event,
				// and it should show up in the snapshot
				Eventually(container.Events).Should(ContainElement("out of memory"))
				Eventually(container.State).Should(Equal(linux_container.StateStopped))

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

				var snapshot linux_container.ContainerSnapshot

				err = json.NewDecoder(out).Decode(&snapshot)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(snapshot.State).Should(Equal("stopped"))
				Ω(snapshot.Events).Should(Equal([]string{"out of memory"}))

				Ω(snapshot.Limits).Should(Equal(
					linux_container.LimitsSnapshot{
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

				var snapshot linux_container.ContainerSnapshot

				err = json.NewDecoder(out).Decode(&snapshot)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(snapshot.Limits).Should(Equal(
					linux_container.LimitsSnapshot{
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
			err := container.Restore(linux_container.ContainerSnapshot{
				State:  "active",
				Events: []string{"out of memory", "foo"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.State()).Should(Equal(linux_container.State("active")))
			Ω(container.Events()).Should(Equal([]string{
				"out of memory",
				"foo",
			}))

		})

		It("restores process state", func() {
			err := container.Restore(linux_container.ContainerSnapshot{
				State:  "active",
				Events: []string{},

				Processes: []linux_container.ProcessSnapshot{
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

			pid, _ := fakeProcessTracker.RestoreArgsForCall(0)
			Ω(pid).Should(Equal(uint32(0)))

			pid, _ = fakeProcessTracker.RestoreArgsForCall(1)
			Ω(pid).Should(Equal(uint32(1)))
		})

		It("makes the next process ID be higher than the highest restored ID", func() {
			err := container.Restore(linux_container.ContainerSnapshot{
				State:  "active",
				Events: []string{},

				Processes: []linux_container.ProcessSnapshot{
					{
						ID:  0,
						TTY: false,
					},
					{
						ID:  5,
						TTY: true,
					},
				},
			})
			Ω(err).ShouldNot(HaveOccurred())

			_, err = container.Run(garden.ProcessSpec{
				Path: "/some/script",
			}, garden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			nextId, _, _, _, _ := fakeProcessTracker.RunArgsForCall(0)

			Ω(nextId).Should(BeNumerically(">", 5))
		})

		It("configures a signaller with the correct pidfile for the process", func() {
			Ω(container.Restore(linux_container.ContainerSnapshot{
				State:  "active",
				Events: []string{},

				Processes: []linux_container.ProcessSnapshot{
					{
						ID:  456,
						TTY: true,
					},
				},
			})).Should(Succeed())

			_, signaller := fakeProcessTracker.RestoreArgsForCall(0)
			Ω(signaller).Should(Equal(&linux_backend.NamespacedSignaller{
				ContainerPath: containerDir,
				Runner:        fakeRunner,
				PidFilePath:   containerDir + "/processes/456.pid",
			}))
		})

		It("restores environment variables", func() {
			err := container.Restore(linux_container.ContainerSnapshot{
				EnvVars: []string{"env1=env1value", "env2=env2Value"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.CurrentEnvVars()).Should(Equal(process.Env{"env1": "env1value", "env2": "env2Value"}))
		})

		It("redoes net-outs", func() {
			Ω(container.Restore(linux_container.ContainerSnapshot{
				NetOuts: []garden.NetOutRule{netOutRule1, netOutRule2},
			})).Should(Succeed())

			Ω(fakeFilter.NetOutCallCount()).Should(Equal(2))
			Ω(fakeFilter.NetOutArgsForCall(0)).Should(Equal(netOutRule1))
			Ω(fakeFilter.NetOutArgsForCall(1)).Should(Equal(netOutRule2))
		})

		Context("when applying a netout rule fails", func() {
			It("returns an error", func() {
				fakeFilter.NetOutReturns(errors.New("didn't work"))

				Ω(container.Restore(
					linux_container.ContainerSnapshot{
						NetOuts: []garden.NetOutRule{{}},
					})).Should(MatchError("didn't work"))
			})
		})

		It("redoes network setup and net-ins", func() {
			err := container.Restore(linux_container.ContainerSnapshot{
				State:  "active",
				Events: []string{},

				NetIns: []linux_container.NetInSpec{
					{
						HostPort:      1234,
						ContainerPort: 5678,
					},
					{
						HostPort:      1235,
						ContainerPort: 5679,
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
			))
		})

		for _, cmd := range []string{"setup", "in"} {
			command := cmd

			Context("when net.sh "+cmd+" fails", func() {
				disaster := errors.New("oh no!")

				JustBeforeEach(func() {
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
					err := container.Restore(linux_container.ContainerSnapshot{
						State:  "active",
						Events: []string{},

						NetIns: []linux_container.NetInSpec{
							{
								HostPort:      1234,
								ContainerPort: 5678,
							},
							{
								HostPort:      1235,
								ContainerPort: 5679,
							},
						},

						NetOuts: []garden.NetOutRule{},
					})
					Ω(err).Should(Equal(disaster))
				})
			})
		}

		It("re-enforces the memory limit", func() {
			err := container.Restore(linux_container.ContainerSnapshot{
				State:  "active",
				Events: []string{},

				Limits: linux_container.LimitsSnapshot{
					Memory: &garden.MemoryLimits{
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
				err := container.Restore(linux_container.ContainerSnapshot{
					State:  "active",
					Events: []string{},
				})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeCgroups.SetValues()).Should(BeEmpty())
			})
		})

		Context("when re-enforcing the memory limit fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeCgroups.WhenSetting("memory", "memory.limit_in_bytes", func() error {
					return disaster
				})
			})

			It("returns the error", func() {
				err := container.Restore(linux_container.ContainerSnapshot{
					State:  "active",
					Events: []string{},

					Limits: linux_container.LimitsSnapshot{
						Memory: &garden.MemoryLimits{
							LimitInBytes: 1024,
						},
					},
				})
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Starting", func() {
		BeforeEach(func() {
			mtu = 1400
		})

		It("executes the container's start.sh with the correct environment", func() {
			err := container.Start()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/start.sh",
					Env: []string{
						"id=some-id",
						"PATH=" + os.Getenv("PATH"),
					},
				},
			))
		})

		It("changes the container's state to active", func() {
			Ω(container.State()).Should(Equal(linux_container.StateBorn))

			err := container.Start()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.State()).Should(Equal(linux_container.StateActive))
		})

		Context("when start.sh fails", func() {
			nastyError := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/start.sh",
					}, func(*exec.Cmd) error {
						return nastyError
					},
				)
			})

			It("returns a wrapped error", func() {
				err := container.Start()
				Ω(err).Should(MatchError("container: start: oh no!"))
			})

			It("does not change the container's state", func() {
				Ω(container.State()).Should(Equal(linux_container.StateBorn))

				err := container.Start()
				Ω(err).Should(HaveOccurred())

				Ω(container.State()).Should(Equal(linux_container.StateBorn))
			})
		})
	})

	Describe("Stopping", func() {
		It("executes the container's stop.sh with the appropriate arguments", func() {
			err := container.Stop(false)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/stop.sh",
				},
			))
		})

		It("sets the container's state to stopped", func() {
			Ω(container.State()).Should(Equal(linux_container.StateBorn))

			err := container.Stop(false)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.State()).Should(Equal(linux_container.StateStopped))

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

			JustBeforeEach(func() {
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
				Ω(container.State()).Should(Equal(linux_container.StateBorn))

				err := container.Stop(false)
				Ω(err).Should(HaveOccurred())

				Ω(container.State()).Should(Equal(linux_container.StateBorn))
			})
		})

		Context("when the container has an oom notifier running", func() {
			JustBeforeEach(func() {
				err := container.LimitMemory(garden.MemoryLimits{
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
		Context("when the container has an oom notifier running", func() {
			JustBeforeEach(func() {
				err := container.LimitMemory(garden.MemoryLimits{
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

		BeforeEach(func() {
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

			JustBeforeEach(func() {
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

			JustBeforeEach(func() {
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

				JustBeforeEach(func() {
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

			JustBeforeEach(func() {
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
		It("delegates to the filter", func() {
			rule := garden.NetOutRule{}
			err := container.NetOut(rule)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeFilter.NetOutCallCount()).Should(Equal(1))
			passedRule := fakeFilter.NetOutArgsForCall(0)
			Ω(passedRule).Should(Equal(rule))
		})

		Context("when the filter fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeFilter.NetOutReturns(disaster)
			})

			It("returns the error", func() {
				err := container.NetOut(garden.NetOutRule{})
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Properties", func() {
		Describe("CRUD", func() {
			It("can get a property", func() {
				value, err := container.GetProperty("property-name")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(value).Should(Equal("property-value"))
			})

			It("can test for a set of properties", func() {
				Ω(container.HasProperties(garden.Properties{
					"other-property": "property-value",
				})).Should(BeFalse())

				Ω(container.HasProperties(garden.Properties{
					"property-name":  "property-value",
					"other-property": "property-value",
				})).Should(BeFalse())

				Ω(container.HasProperties(garden.Properties{
					"property-name": "property-value",
				})).Should(BeTrue())
			})

			It("returns an error when the property is undefined", func() {
				_, err := container.GetProperty("some-other-property")
				Ω(err).Should(Equal(linux_container.UndefinedPropertyError{"some-other-property"}))
				Ω(err).Should(MatchError("property does not exist: some-other-property"))
			})

			It("can set a new property", func() {
				err := container.SetProperty("some-other-property", "some-other-value")
				Ω(err).ShouldNot(HaveOccurred())
				value, err := container.GetProperty("some-other-property")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(value).Should(Equal("some-other-value"))
			})

			It("can override an existing property", func() {
				err := container.SetProperty("property-name", "some-other-new-value")
				Ω(err).ShouldNot(HaveOccurred())

				value, err := container.GetProperty("property-name")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(value).Should(Equal("some-other-new-value"))
			})

			Context("when removing a property", func() {
				var err error

				JustBeforeEach(func() {
					err = container.SetProperty("other-property-name", "some-other-value")
					Ω(err).ShouldNot(HaveOccurred())

					err = container.RemoveProperty("property-name")
				})

				It("removes the property", func() {
					Ω(err).ShouldNot(HaveOccurred())

					_, err = container.GetProperty("property-name")
					Ω(err).Should(Equal(linux_container.UndefinedPropertyError{"property-name"}))
				})

				It("does not remove other properties", func() {
					value, err := container.GetProperty("other-property-name")
					Ω(err).ShouldNot(HaveOccurred())
					Ω(value).Should(Equal("some-other-value"))
				})
			})

			It("returns an error when removing an undefined property", func() {
				err := container.RemoveProperty("some-other-property")
				Ω(err).Should(HaveOccurred())
				Ω(err).Should(MatchError("property does not exist: some-other-property"))
			})
		})

		It("can return all properties as a map", func() {
			properties, err := container.GetProperties()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(properties).Should(Equal(garden.Properties{"property-name": "property-value"}))
		})

		It("returns a properties snapshot", func() {
			err := container.SetProperty("some-property", "some-value")
			Ω(err).ShouldNot(HaveOccurred())

			properties := container.Properties()
			Ω(properties["some-property"]).Should(Equal("some-value"))

			err = container.SetProperty("some-property", "some-other-value")
			Ω(err).ShouldNot(HaveOccurred())

			Ω(properties["some-property"]).Should(Equal("some-value"))
		})

		Context("with a nil map of properties at container creation", func() {
			BeforeEach(func() {
				containerProps = nil
			})

			It("reading a property fails in the expected way", func() {
				_, err := container.GetProperty("property-name")
				Ω(err).Should(Equal(linux_container.UndefinedPropertyError{"property-name"}))
			})

			It("setting a property succeeds", func() {
				err := container.SetProperty("some-other-property", "some-other-value")
				Ω(err).ShouldNot(HaveOccurred())

				value, err := container.GetProperty("some-other-property")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(value).Should(Equal("some-other-value"))
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

			Ω(info.HostIP).Should(Equal("2.3.4.2"))
			Ω(info.ContainerIP).Should(Equal("1.2.3.4"))
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
			Ω(info.MappedPorts).Should(Equal([]garden.PortMapping{
				{HostPort: 1234, ContainerPort: 5678},
				{HostPort: 1235, ContainerPort: 5679},
			}))

		})

		Context("with running processes", func() {
			JustBeforeEach(func() {
				p1 := new(wfakes.FakeProcess)
				p1.IDReturns(1)

				p2 := new(wfakes.FakeProcess)
				p2.IDReturns(2)

				p3 := new(wfakes.FakeProcess)
				p3.IDReturns(3)

				fakeProcessTracker.ActiveProcessesReturns([]garden.Process{p1, p2, p3})
			})

			It("returns their process IDs", func() {
				info, err := container.Info()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(info.ProcessIDs).Should(Equal([]uint32{1, 2, 3}))
			})
		})
	})
})

func uint64ptr(n uint64) *uint64 {
	return &n
}
