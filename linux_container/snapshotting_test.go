package linux_container_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker/fake_process_tracker"
	wfakes "github.com/cloudfoundry-incubator/garden/fakes"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var _ = Describe("Linux containers", func() {
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

		containerProps = map[string]string{
			"property-name": "property-value",
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
})
