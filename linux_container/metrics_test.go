package linux_container_test

import (
	"errors"
	"net"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/bandwidth_manager/fake_bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/fake_network_statisticser"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/fake_quota_manager"
	networkFakes "github.com/cloudfoundry-incubator/garden-linux/network/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/port_pool/fake_port_pool"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker/fake_process_tracker"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Linux containers", func() {
	var fakeCgroups *fake_cgroups_manager.FakeCgroupsManager
	var fakeQuotaManager *fake_quota_manager.FakeQuotaManager
	var fakeNetStats *fake_network_statisticser.FakeNetworkStatisticser
	var container *linux_container.LinuxContainer
	var containerDir string

	BeforeEach(func() {
		fakeCgroups = fake_cgroups_manager.New("/cgroups", "some-id")
		fakeQuotaManager = new(fake_quota_manager.FakeQuotaManager)
		fakeNetStats = new(fake_network_statisticser.FakeNetworkStatisticser)
	})

	JustBeforeEach(func() {
		_, subnet, _ := net.ParseCIDR("2.3.4.0/30")
		containerResources := linux_backend.NewResources(
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
			fake_command_runner.New(),
			fakeCgroups,
			fakeQuotaManager,
			fake_bandwidth_manager.New(),
			new(fake_process_tracker.FakeProcessTracker),
			new(networkFakes.FakeFilter),
			fakeNetStats,
			lagertest.NewTestLogger("linux-container-limits-test"),
		)
	})

	Describe("Metrics", func() {
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
				metrics, err := container.Metrics()
				Expect(err).ToNot(HaveOccurred())
				Expect(metrics.MemoryStat).To(Equal(garden.ContainerMemoryStat{
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
					TotalUsageTowardLimit:   (17 + 16 - 26),
				}))

			})
		})

		Context("when getting memory.stat fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeCgroups.WhenGetting("memory", "memory.stat", func() (string, error) {
					return "", disaster
				})
			})

			It("returns an error", func() {
				_, err := container.Metrics()
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
				metrics, err := container.Metrics()
				Expect(err).ToNot(HaveOccurred())
				Expect(metrics.CPUStat).To(Equal(garden.ContainerCPUStat{
					Usage:  42,
					User:   1,
					System: 2,
				}))

			})
		})

		Context("when getting cpuacct/cpuacct.usage fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeCgroups.WhenGetting("cpuacct", "cpuacct.usage", func() (string, error) {
					return "", disaster
				})
			})

			It("returns an error", func() {
				_, err := container.Metrics()
				Expect(err).To(Equal(disaster))
			})
		})

		Context("when getting cpuacct/cpuacct.stat fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeCgroups.WhenGetting("cpuacct", "cpuacct.stat", func() (string, error) {
					return "", disaster
				})
			})

			It("returns an error", func() {
				_, err := container.Metrics()
				Expect(err).To(Equal(disaster))
			})
		})

		Describe("disk usage info", func() {
			It("is returned in the response", func() {
				fakeQuotaManager.GetUsageReturns(garden.ContainerDiskStat{
					TotalBytesUsed:      1,
					TotalInodesUsed:     2,
					ExclusiveBytesUsed:  3,
					ExclusiveInodesUsed: 4,
				}, nil)

				metrics, err := container.Metrics()
				Expect(err).ToNot(HaveOccurred())

				Expect(metrics.DiskStat).To(Equal(garden.ContainerDiskStat{
					TotalBytesUsed:      1,
					TotalInodesUsed:     2,
					ExclusiveBytesUsed:  3,
					ExclusiveInodesUsed: 4,
				}))

			})

			Context("when getting the disk usage fails", func() {
				disaster := errors.New("oh no!")

				JustBeforeEach(func() {
					fakeQuotaManager.GetUsageReturns(garden.ContainerDiskStat{}, disaster)
				})

				It("returns the error", func() {
					_, err := container.Metrics()
					Expect(err).To(Equal(disaster))
				})
			})
		})

		Describe("Getting network info", func() {
			Context("on existing interface", func() {
				It("it returns container statistics, which are the inverse of the returned values", func() {
					fakeNetStats.StatisticsReturns(garden.ContainerNetworkStat{
						RxBytes: 2,
						TxBytes: 1,
					}, nil)

					metrics, err := container.Metrics()
					Expect(err).ToNot(HaveOccurred())

					Expect(metrics.NetworkStat).To(Equal(garden.ContainerNetworkStat{
						RxBytes: 1, // the network statisticser returns the host-side stats
						TxBytes: 2, // therefore the container should have reversed them
					}))
				})
			})

			Context("on non-existent interface", func() {
				JustBeforeEach(func() {
					fakeNetStats.StatisticsReturns(garden.ContainerNetworkStat{}, errors.New("link does not exist"))
				})

				It("returns zero-ed out network stats", func() {
					metrics, err := container.Metrics()
					Expect(err).ToNot(HaveOccurred())
					Expect(metrics.NetworkStat.RxBytes).To(Equal(uint64(0)))
					Expect(metrics.NetworkStat.TxBytes).To(Equal(uint64(0)))
				})
			})
		})
	})
})
