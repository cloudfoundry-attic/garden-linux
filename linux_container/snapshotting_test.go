package linux_container_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"os/exec"
	"strconv"
	"time"

	"github.com/blang/semver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/bandwidth_manager/fake_bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/fake_iptables_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/fake_network_statisticser"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/fake_quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/fake_watcher"
	networkFakes "github.com/cloudfoundry-incubator/garden-linux/network/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/port_pool/fake_port_pool"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker/fake_process_tracker"
	wfakes "github.com/cloudfoundry-incubator/garden/fakes"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var _ = Describe("Linux containers", func() {
	var (
		fakeCgroups          *fake_cgroups_manager.FakeCgroupsManager
		fakeQuotaManager     *fake_quota_manager.FakeQuotaManager
		fakeBandwidthManager *fake_bandwidth_manager.FakeBandwidthManager
		fakeRunner           *fake_command_runner.FakeCommandRunner
		containerResources   *linux_backend.Resources
		container            *linux_container.LinuxContainer
		fakePortPool         *fake_port_pool.FakePortPool
		fakeProcessTracker   *fake_process_tracker.FakeProcessTracker
		fakeFilter           *networkFakes.FakeFilter
		fakeOomWatcher       *fake_watcher.FakeWatcher
		containerDir         string
		containerProps       map[string]string
		containerVersion     semver.Version
		fakeIPTablesManager  *fake_iptables_manager.FakeIPTablesManager
	)

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

		fakeQuotaManager = new(fake_quota_manager.FakeQuotaManager)
		fakeBandwidthManager = fake_bandwidth_manager.New()
		fakeProcessTracker = new(fake_process_tracker.FakeProcessTracker)
		fakeFilter = new(networkFakes.FakeFilter)

		fakePortPool = fake_port_pool.New(1000)

		var err error
		containerDir, err = ioutil.TempDir("", "depot")
		Expect(err).ToNot(HaveOccurred())

		containerVersion = semver.Version{Major: 1, Minor: 0, Patch: 0}

		_, subnet, err := net.ParseCIDR("2.3.4.0/30")
		containerResources = linux_backend.NewResources(
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

		fakeIPTablesManager = new(fake_iptables_manager.FakeIPTablesManager)
	})

	fakeOomWatcher = new(fake_watcher.FakeWatcher)

	JustBeforeEach(func() {
		container = linux_container.NewLinuxContainer(
			linux_backend.LinuxContainerSpec{
				ID:                  "some-id",
				ContainerPath:       containerDir,
				ContainerRootFSPath: "some-rootfs-path",
				Resources:           containerResources,
				State:               linux_backend.StateBorn,
				ContainerSpec: garden.ContainerSpec{
					Handle:     "some-handle",
					GraceTime:  time.Second * 1,
					Env:        []string{"env1=env1Value", "env2=env2Value"},
					Properties: containerProps,
				},
				Version: containerVersion,
			},
			fakePortPool,
			fakeRunner,
			fakeCgroups,
			fakeQuotaManager,
			fakeBandwidthManager,
			fakeProcessTracker,
			fakeFilter,
			fakeIPTablesManager,
			new(fake_network_statisticser.FakeNetworkStatisticser),
			fakeOomWatcher,
			lagertest.NewTestLogger("linux-container-limits-test"),
		)
	})

	Describe("Snapshotting", func() {
		memoryLimits := garden.MemoryLimits{
			LimitInBytes: 1,
		}

		diskLimits := garden.DiskLimits{
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
			p1.IDReturns("1")

			p2 := new(wfakes.FakeProcess)
			p2.IDReturns("2")

			p3 := new(wfakes.FakeProcess)
			p3.IDReturns("3")

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
			Expect(snapshot.RootFSPath).To(Equal("some-rootfs-path"))

			Expect(snapshot.GraceTime).To(Equal(1 * time.Second))

			Expect(snapshot.State).To(Equal("active"))

			_, subnet, err := net.ParseCIDR("2.3.4.0/30")
			Expect(snapshot.Resources).To(Equal(
				linux_container.ResourcesSnapshot{
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

			Expect(snapshot.NetOuts).To(Equal([]garden.NetOutRule{
				netOutRule1, netOutRule2,
			}))

			Expect(snapshot.Processes).To(ContainElement(
				linux_backend.ActiveProcess{
					ID: 1,
				},
			))

			Expect(snapshot.Processes).To(ContainElement(
				linux_backend.ActiveProcess{
					ID: 2,
				},
			))

			Expect(snapshot.Processes).To(ContainElement(
				linux_backend.ActiveProcess{
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
				fakeOomWatcher.WatchStub = func(onOom func()) error {
					onOom()
					return nil
				}

				err := container.LimitMemory(memoryLimits)
				Expect(err).ToNot(HaveOccurred())

				Eventually(container.Events).Should(ContainElement("out of memory"))
				Eventually(container.State).Should(Equal(linux_backend.StateStopped))

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

				var snapshot linux_backend.LinuxContainerSpec

				err = json.NewDecoder(out).Decode(&snapshot)
				Expect(err).ToNot(HaveOccurred())

				Expect(snapshot.State).To(Equal(linux_backend.StateStopped))
				Expect(snapshot.Events).To(Equal([]string{"out of memory"}))

				Expect(snapshot.Limits).To(Equal(
					linux_backend.Limits{
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

				var snapshot linux_backend.LinuxContainerSpec

				err = json.NewDecoder(out).Decode(&snapshot)
				Expect(err).ToNot(HaveOccurred())

				Expect(snapshot.Limits).To(Equal(
					linux_backend.Limits{
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
			err := container.Restore(linux_backend.LinuxContainerSpec{
				State:     "active",
				Events:    []string{"out of memory", "foo"},
				Resources: containerResources,
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(container.State()).To(Equal(linux_backend.State("active")))
			Expect(container.Events()).To(Equal([]string{
				"out of memory",
				"foo",
			}))

		})

		It("restores process state", func() {
			err := container.Restore(linux_backend.LinuxContainerSpec{
				State:     "active",
				Events:    []string{},
				Resources: containerResources,

				Processes: []linux_backend.ActiveProcess{
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
			Expect(pid).To(Equal("0"))

			pid, _ = fakeProcessTracker.RestoreArgsForCall(1)
			Expect(pid).To(Equal("1"))
		})

		It("makes the next process ID be higher than the highest restored ID", func() {
			err := container.Restore(linux_backend.LinuxContainerSpec{
				State:     "active",
				Events:    []string{},
				Resources: containerResources,

				Processes: []linux_backend.ActiveProcess{
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
				User: "alice",
				Path: "/some/script",
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			nextGuid, _, _, _, _ := fakeProcessTracker.RunArgsForCall(0)
			nextId, _ := strconv.Atoi(nextGuid)

			Expect(nextId).To(BeNumerically(">", 5))
		})

		It("restores the correct process signaller", func() {
			Expect(container.Restore(linux_backend.LinuxContainerSpec{
				State:     "active",
				Processes: []linux_backend.ActiveProcess{{ID: 0, TTY: false}},
				Resources: containerResources,
			})).To(Succeed())

			_, signaller := fakeProcessTracker.RestoreArgsForCall(0)
			Expect(signaller).To(BeAssignableToTypeOf(&process_tracker.LinkSignaller{}))
		})

		Context("when the container is version 0.0.0 (old container)", func() {
			BeforeEach(func() {
				containerVersion = semver.Version{Major: 0, Minor: 0, Patch: 0}
			})

			It("restores the correct process signaller", func() {
				Expect(container.Restore(linux_backend.LinuxContainerSpec{
					State:     "active",
					Processes: []linux_backend.ActiveProcess{{ID: 0, TTY: false}},
					Resources: containerResources,
				})).To(Succeed())

				_, signaller := fakeProcessTracker.RestoreArgsForCall(0)
				Expect(signaller).To(BeAssignableToTypeOf(&process_tracker.NamespacedSignaller{}))
			})
		})

		It("restores environment variables", func() {
			err := container.Restore(linux_backend.LinuxContainerSpec{
				ContainerSpec: garden.ContainerSpec{Env: []string{"env1=env1value", "env2=env2Value"}},
				Resources:     containerResources,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(container.Env).To(Equal([]string{"env1=env1value", "env2=env2Value"}))
		})

		It("redoes net-outs", func() {
			Expect(container.Restore(linux_backend.LinuxContainerSpec{
				NetOuts:   []garden.NetOutRule{netOutRule1, netOutRule2},
				Resources: containerResources,
			})).To(Succeed())

			Expect(fakeFilter.NetOutCallCount()).To(Equal(2))
			Expect(fakeFilter.NetOutArgsForCall(0)).To(Equal(netOutRule1))
			Expect(fakeFilter.NetOutArgsForCall(1)).To(Equal(netOutRule2))
		})

		Context("when applying a netout rule fails", func() {
			It("returns an error", func() {
				fakeFilter.NetOutReturns(errors.New("didn't work"))

				Expect(container.Restore(
					linux_backend.LinuxContainerSpec{
						NetOuts:   []garden.NetOutRule{{}},
						Resources: containerResources,
					})).To(MatchError("didn't work"))
			})
		})

		It("redoes network setup and net-ins", func() {
			err := container.Restore(linux_backend.LinuxContainerSpec{
				State:     "active",
				Events:    []string{},
				Resources: containerResources,

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
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
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

		It("should redo iptables setup", func() {
			err := container.Restore(linux_backend.LinuxContainerSpec{
				ID:        "test-container",
				State:     "active",
				Events:    []string{},
				Resources: containerResources,

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
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeIPTablesManager.ContainerSetupCallCount()).To(Equal(1))
			containerID, bridgeName, ip, network := fakeIPTablesManager.ContainerSetupArgsForCall(0)
			Expect(containerID).To(Equal("test-container"))
			Expect(bridgeName).To(Equal("some-bridge"))
			Expect(ip.String()).To(Equal("1.2.3.4"))
			Expect(network.String()).To(Equal("2.3.4.0/30"))
		})

		for _, cmd := range []string{"in"} {
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
					err := container.Restore(linux_backend.LinuxContainerSpec{
						State:     "active",
						Events:    []string{},
						Resources: containerResources,

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

						NetOuts: []garden.NetOutRule{},
					})
					Expect(err).To(Equal(disaster))
				})
			})
		}

		Context("when iptables manager returns an error", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeIPTablesManager.ContainerSetupReturns(disaster)
			})

			It("should return the error", func() {
				err := container.Restore(linux_backend.LinuxContainerSpec{
					State:     "active",
					Events:    []string{},
					Resources: containerResources,
				})
				Expect(err).To(Equal(disaster))
			})
		})

		It("re-enforces the memory limit", func() {
			fakeOomWatcher.WatchStub = func(onOom func()) error {
				onOom()
				return nil
			}

			err := container.Restore(linux_backend.LinuxContainerSpec{
				State:     "active",
				Events:    []string{},
				Resources: containerResources,

				Limits: linux_backend.Limits{
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
				err := container.Restore(linux_backend.LinuxContainerSpec{
					State:     "active",
					Events:    []string{},
					Resources: containerResources,
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
				err := container.Restore(linux_backend.LinuxContainerSpec{
					State:     "active",
					Events:    []string{},
					Resources: containerResources,

					Limits: linux_backend.Limits{
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
