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
		Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			_, _, err = container.NetIn(1, 2)
			Expect(err).ToNot(HaveOccurred())

			_, _, err = container.NetIn(3, 4)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			var snapshot linux_container.ContainerSnapshot

			err = json.NewDecoder(out).Decode(&snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(snapshot.ID).To(Equal("some-id"))
			Expect(snapshot.Handle).To(Equal("some-handle"))

			Expect(snapshot.GraceTime).To(Equal(1 * time.Second))

			Expect(snapshot.State).To(Equal("active"))

			_, subnet, err := net.ParseCIDR("2.3.4.0/30")
			Expect(snapshot.Resources).To(Equal(
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

			Expect(snapshot.NetIns).To(Equal(
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

			Expect(snapshot.NetOuts).To(Equal([]garden.NetOutRule{
				netOutRule1, netOutRule2,
			}))

			Expect(snapshot.Processes).To(ContainElement(
				linux_container.ProcessSnapshot{
					ID: 1,
				},
			))

			Expect(snapshot.Processes).To(ContainElement(
				linux_container.ProcessSnapshot{
					ID: 2,
				},
			))

			Expect(snapshot.Processes).To(ContainElement(
				linux_container.ProcessSnapshot{
					ID: 3,
				},
			))

			Expect(snapshot.Properties).To(Equal(garden.Properties(map[string]string{
				"property-name": "property-value",
			})))

			Expect(snapshot.EnvVars).To(Equal([]string{"env1=env1Value", "env2=env2Value"}))
		})

		Context("with limits set", func() {
			JustBeforeEach(func() {
				err := container.LimitMemory(memoryLimits)
				Expect(err).ToNot(HaveOccurred())

				// oom exits immediately since it's faked out; should see event,
				// and it should show up in the snapshot
				Eventually(container.Events).Should(ContainElement("out of memory"))
				Eventually(container.State).Should(Equal(linux_container.StateStopped))

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

				var snapshot linux_container.ContainerSnapshot

				err = json.NewDecoder(out).Decode(&snapshot)
				Expect(err).ToNot(HaveOccurred())

				Expect(snapshot.State).To(Equal("stopped"))
				Expect(snapshot.Events).To(Equal([]string{"out of memory"}))

				Expect(snapshot.Limits).To(Equal(
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
				Expect(err).ToNot(HaveOccurred())

				var snapshot linux_container.ContainerSnapshot

				err = json.NewDecoder(out).Decode(&snapshot)
				Expect(err).ToNot(HaveOccurred())

				Expect(snapshot.Limits).To(Equal(
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
			Expect(err).ToNot(HaveOccurred())

			Expect(container.State()).To(Equal(linux_container.State("active")))
			Expect(container.Events()).To(Equal([]string{
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
			Expect(err).ToNot(HaveOccurred())

			pid, _ := fakeProcessTracker.RestoreArgsForCall(0)
			Expect(pid).To(Equal(uint32(0)))

			pid, _ = fakeProcessTracker.RestoreArgsForCall(1)
			Expect(pid).To(Equal(uint32(1)))
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
			Expect(err).ToNot(HaveOccurred())

			_, err = container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/some/script",
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			nextId, _, _, _, _ := fakeProcessTracker.RunArgsForCall(0)

			Expect(nextId).To(BeNumerically(">", 5))
		})

		It("configures a signaller with the correct pidfile for the process", func() {
			Expect(container.Restore(linux_container.ContainerSnapshot{
				State:  "active",
				Events: []string{},

				Processes: []linux_container.ProcessSnapshot{
					{
						ID:  456,
						TTY: true,
					},
				},
			})).To(Succeed())

			_, signaller := fakeProcessTracker.RestoreArgsForCall(0)
			Expect(signaller).To(Equal(&linux_backend.NamespacedSignaller{
				ContainerPath: containerDir,
				Runner:        fakeRunner,
				PidFilePath:   containerDir + "/processes/456.pid",
			}))
		})

		It("restores environment variables", func() {
			err := container.Restore(linux_container.ContainerSnapshot{
				EnvVars: []string{"env1=env1value", "env2=env2Value"},
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(container.CurrentEnvVars()).To(Equal(process.Env{"env1": "env1value", "env2": "env2Value"}))
		})

		It("redoes net-outs", func() {
			Expect(container.Restore(linux_container.ContainerSnapshot{
				NetOuts: []garden.NetOutRule{netOutRule1, netOutRule2},
			})).To(Succeed())

			Expect(fakeFilter.NetOutCallCount()).To(Equal(2))
			Expect(fakeFilter.NetOutArgsForCall(0)).To(Equal(netOutRule1))
			Expect(fakeFilter.NetOutArgsForCall(1)).To(Equal(netOutRule2))
		})

		Context("when applying a netout rule fails", func() {
			It("returns an error", func() {
				fakeFilter.NetOutReturns(errors.New("didn't work"))

				Expect(container.Restore(
					linux_container.ContainerSnapshot{
						NetOuts: []garden.NetOutRule{{}},
					})).To(MatchError("didn't work"))
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
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
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
					Expect(err).To(Equal(disaster))
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
				err := container.Restore(linux_container.ContainerSnapshot{
					State:  "active",
					Events: []string{},
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeCgroups.SetValues()).To(BeEmpty())
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
				Expect(err).To(Equal(disaster))
			})
		})
	})
})
