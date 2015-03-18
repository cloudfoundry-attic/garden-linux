package linux_container_test

import (
	"errors"
	"io/ioutil"
	"math"
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
	var containerDir string

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeCgroups = fake_cgroups_manager.New("/cgroups", "some-id")

		fakeQuotaManager = fake_quota_manager.New()
		fakeBandwidthManager = fake_bandwidth_manager.New()

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
			fakeCgroups,
			fakeQuotaManager,
			fakeBandwidthManager,
			new(fake_process_tracker.FakeProcessTracker),
			process.Env{"env1": "env1Value", "env2": "env2Value"},
			new(networkFakes.FakeFilter),
		)
	})

	Describe("Limiting bandwidth", func() {
		limits := garden.BandwidthLimits{
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
		limits := garden.BandwidthLimits{
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

				JustBeforeEach(func() {
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
			limits := garden.MemoryLimits{
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
			limits := garden.MemoryLimits{
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

				limits := garden.MemoryLimits{
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
			JustBeforeEach(func() {
				fakeRunner.WhenWaitingFor(fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/oom",
				}, func(cmd *exec.Cmd) error {
					return nil
				})
			})

			It("stops the container", func() {
				limits := garden.MemoryLimits{
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
				limits := garden.MemoryLimits{
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

			JustBeforeEach(func() {
				fakeCgroups.WhenSetting("memory", "memory.memsw.limit_in_bytes", func() error {
					return disaster
				})
			})

			It("does not fail", func() {
				err := container.LimitMemory(garden.MemoryLimits{
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

				err := container.LimitMemory(garden.MemoryLimits{
					LimitInBytes: 102400,
				})

				Ω(err).ShouldNot(HaveOccurred())
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
				err := container.LimitMemory(garden.MemoryLimits{
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
			It("returns the error", func() {
				disaster := errors.New("oh no!")
				fakeCgroups.WhenGetting("memory", "memory.limit_in_bytes", func() (string, error) {
					return "", disaster
				})

				_, err := container.CurrentMemoryLimits()
				Ω(err).Should(Equal(disaster))
			})
		})

		Context("when the returned memory limit is malformed", func() {
			It("returns the error", func() {
				fakeCgroups.WhenGetting("memory", "memory.limit_in_bytes", func() (string, error) {
					return "500M", nil
				})

				_, err := container.CurrentMemoryLimits()
				Ω(err.Error()).Should(HaveSuffix("invalid syntax"))
			})
		})

	})

	Describe("Limiting CPU", func() {
		It("sets cpu.shares", func() {
			limits := garden.CPULimits{
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
				err := container.LimitCPU(garden.CPULimits{
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
			It("returns the error", func() {
				disaster := errors.New("oh no!")
				fakeCgroups.WhenGetting("cpu", "cpu.shares", func() (string, error) {
					return "", disaster
				})

				_, err := container.CurrentCPULimits()
				Ω(err).Should(Equal(disaster))
			})
		})

		Context("when the current CPU limit is malformed", func() {
			It("returns the error", func() {
				fakeCgroups.WhenGetting("cpu", "cpu.shares", func() (string, error) {
					return "50%", nil
				})

				_, err := container.CurrentCPULimits()
				Ω(err.Error()).Should(HaveSuffix("invalid syntax"))
			})
		})
	})

	Describe("Limiting disk", func() {
		limits := garden.DiskLimits{
			BlockSoft: 3,
			BlockHard: 4,

			InodeSoft: 13,
			InodeHard: 14,

			ByteSoft: 23,
			ByteHard: 24,
		}

		It("sets the quota via the quota manager with the uid and limits", func() {
			resultingLimits := garden.DiskLimits{
				BlockHard: 1234567,
			}

			fakeQuotaManager.GetLimitsResult = resultingLimits

			err := container.LimitDisk(limits)
			Ω(err).ShouldNot(HaveOccurred())

			uid := containerResources.UserUID

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
			limits := garden.DiskLimits{
				BlockHard: 1234567,
			}

			fakeQuotaManager.GetLimitsResult = limits

			receivedLimits, err := container.CurrentDiskLimits()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(receivedLimits).Should(Equal(limits))
		})

		Context("when getting the limit fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeQuotaManager.GetLimitsError = disaster
			})

			It("returns the error", func() {
				limits, err := container.CurrentDiskLimits()
				Ω(err).Should(Equal(disaster))
				Ω(limits).Should(BeZero())
			})
		})
	})
})
