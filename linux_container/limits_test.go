package linux_container_test

import (
	"errors"
	"io/ioutil"
	"math"
	"net"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"code.cloudfoundry.org/lager/lagertest"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden-linux/linux_backend"
	"code.cloudfoundry.org/garden-linux/linux_container"
	"code.cloudfoundry.org/garden-linux/linux_container/bandwidth_manager/fake_bandwidth_manager"
	"code.cloudfoundry.org/garden-linux/linux_container/cgroups_manager/fake_cgroups_manager"
	"code.cloudfoundry.org/garden-linux/linux_container/fake_iptables_manager"
	"code.cloudfoundry.org/garden-linux/linux_container/fake_network_statisticser"
	"code.cloudfoundry.org/garden-linux/linux_container/fake_quota_manager"
	"code.cloudfoundry.org/garden-linux/linux_container/fake_watcher"
	networkFakes "code.cloudfoundry.org/garden-linux/network/fakes"
	"code.cloudfoundry.org/garden-linux/port_pool/fake_port_pool"
	"code.cloudfoundry.org/garden-linux/process_tracker/fake_process_tracker"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var _ = Describe("Linux containers", func() {
	var fakeCgroups *fake_cgroups_manager.FakeCgroupsManager
	var fakeQuotaManager *fake_quota_manager.FakeQuotaManager
	var fakeBandwidthManager *fake_bandwidth_manager.FakeBandwidthManager
	var fakeRunner *fake_command_runner.FakeCommandRunner
	var fakeOomWatcher *fake_watcher.FakeWatcher
	var containerResources *linux_backend.Resources
	var container *linux_container.LinuxContainer
	var containerDir string

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeCgroups = fake_cgroups_manager.New("/cgroups", "some-id")

		fakeQuotaManager = new(fake_quota_manager.FakeQuotaManager)
		fakeBandwidthManager = fake_bandwidth_manager.New()
		fakeOomWatcher = new(fake_watcher.FakeWatcher)

		var err error
		containerDir, err = ioutil.TempDir("", "depot")
		Expect(err).ToNot(HaveOccurred())

		_, subnet, _ := net.ParseCIDR("2.3.4.0/30")
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
	})

	JustBeforeEach(func() {
		container = linux_container.NewLinuxContainer(
			linux_backend.LinuxContainerSpec{
				ID:                  "some-id",
				ContainerPath:       containerDir,
				ContainerRootFSPath: "some-volume-path",
				Resources:           containerResources,
				ContainerSpec: garden.ContainerSpec{
					Handle:    "some-handle",
					GraceTime: time.Second * 1,
				},
			},
			fake_port_pool.New(1000),
			fakeRunner,
			fakeCgroups,
			fakeQuotaManager,
			fakeBandwidthManager,
			new(fake_process_tracker.FakeProcessTracker),
			new(networkFakes.FakeFilter),
			new(fake_iptables_manager.FakeIPTablesManager),
			new(fake_network_statisticser.FakeNetworkStatisticser),
			fakeOomWatcher,
			lagertest.NewTestLogger("linux-container-limits-test"),
		)
	})

	Describe("Limiting bandwidth", func() {
		limits := garden.BandwidthLimits{
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
		limits := garden.BandwidthLimits{
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

				JustBeforeEach(func() {
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
			limits := garden.MemoryLimits{
				LimitInBytes: 102400,
			}

			err := container.LimitMemory(limits)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeOomWatcher.WatchCallCount()).To(Equal(1))
		})

		It("sets memory.limit_in_bytes and then memory.memsw.limit_in_bytes", func() {
			limits := garden.MemoryLimits{
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

		Context("when the OOM watcher calls back", func() {
			BeforeEach(func() {
				fakeOomWatcher.WatchStub = func(onOom func()) error {
					onOom()
					return nil
				}
			})

			It("stops the container", func() {
				limits := garden.MemoryLimits{
					LimitInBytes: 102400,
				}

				err := container.LimitMemory(limits)
				Expect(err).ToNot(HaveOccurred())

				Eventually(fakeRunner).Should(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/stop.sh",
					},
				))
			})

			It("registers an 'out of memory' event", func() {
				limits := garden.MemoryLimits{
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

			JustBeforeEach(func() {
				fakeCgroups.WhenSetting("memory", "memory.memsw.limit_in_bytes", func() error {
					return disaster
				})
			})

			It("does not fail", func() {
				err := container.LimitMemory(garden.MemoryLimits{
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

				err := container.LimitMemory(garden.MemoryLimits{
					LimitInBytes: 102400,
				})

				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when setting memory.limit_in_bytes fails the second time", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
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
				err := container.LimitMemory(garden.MemoryLimits{
					LimitInBytes: 102400,
				})

				Expect(err).To(Equal(disaster))
			})
		})

		Context("when starting the oom notifier fails", func() {
			BeforeEach(func() {
				fakeOomWatcher.WatchReturns(errors.New("banana"))
			})

			It("returns the error", func() {
				err := container.LimitMemory(garden.MemoryLimits{
					LimitInBytes: 102400,
				})

				Expect(err).To(MatchError("banana"))
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
			It("returns the error", func() {
				disaster := errors.New("oh no!")
				fakeCgroups.WhenGetting("memory", "memory.limit_in_bytes", func() (string, error) {
					return "", disaster
				})

				_, err := container.CurrentMemoryLimits()
				Expect(err).To(Equal(disaster))
			})
		})

		Context("when the returned memory limit is malformed", func() {
			It("returns the error", func() {
				fakeCgroups.WhenGetting("memory", "memory.limit_in_bytes", func() (string, error) {
					return "500M", nil
				})

				_, err := container.CurrentMemoryLimits()
				Expect(err.Error()).To(HaveSuffix("invalid syntax"))
			})
		})

	})

	Describe("Limiting CPU", func() {
		It("sets cpu.shares", func() {
			limits := garden.CPULimits{
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
				err := container.LimitCPU(garden.CPULimits{
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
			It("returns the error", func() {
				disaster := errors.New("oh no!")
				fakeCgroups.WhenGetting("cpu", "cpu.shares", func() (string, error) {
					return "", disaster
				})

				_, err := container.CurrentCPULimits()
				Expect(err).To(Equal(disaster))
			})
		})

		Context("when the current CPU limit is malformed", func() {
			It("returns the error", func() {
				fakeCgroups.WhenGetting("cpu", "cpu.shares", func() (string, error) {
					return "50%", nil
				})

				_, err := container.CurrentCPULimits()
				Expect(err.Error()).To(HaveSuffix("invalid syntax"))
			})
		})
	})

	Describe("Limiting disk", func() {
		limits := garden.DiskLimits{
			InodeSoft: 13,
			InodeHard: 14,

			ByteSoft: 23,
			ByteHard: 24,
		}

		It("sets the quota via the quota manager with the container id", func() {
			err := container.LimitDisk(limits)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeQuotaManager.SetLimitsCallCount()).To(Equal(1))
			_, path, receivedLimits := fakeQuotaManager.SetLimitsArgsForCall(0)
			Expect(path).To(Equal(container.RootFSPath()))
			Expect(receivedLimits).To(Equal(limits))
		})

		Context("when setting the quota fails", func() {
			It("returns the error", func() {
				disaster := errors.New("oh no!")
				fakeQuotaManager.SetLimitsReturns(disaster)

				err := container.LimitDisk(limits)
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Getting the current disk limits", func() {
		It("returns the disk limits", func() {
			limits := garden.DiskLimits{
				ByteHard: 1234567,
			}

			fakeQuotaManager.GetLimitsReturns(limits, nil)

			receivedLimits, err := container.CurrentDiskLimits()
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedLimits).To(Equal(limits))
		})

		Context("when getting the limit fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeQuotaManager.GetLimitsReturns(garden.DiskLimits{}, disaster)
			})

			It("returns the error", func() {
				limits, err := container.CurrentDiskLimits()
				Expect(err).To(Equal(disaster))
				Expect(limits).To(BeZero())
			})
		})
	})
})
