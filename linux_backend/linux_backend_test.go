package linux_backend_test

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_repository"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/system_info/fake_system_info"
)

var _ = Describe("LinuxBackend", func() {
	var logger *lagertest.TestLogger

	var fakeResourcePool *fakes.FakeResourcePool
	var fakeSystemInfo *fake_system_info.FakeProvider
	var fakeContainerProvider *fakes.FakeContainerProvider
	var containerRepo linux_backend.ContainerRepository
	var linuxBackend *linux_backend.LinuxBackend
	var snapshotsPath string
	var maxContainers int
	var fakeContainers map[string]*fakes.FakeContainer

	newTestContainer := func(spec linux_backend.LinuxContainerSpec) *fakes.FakeContainer {
		container := new(fakes.FakeContainer)
		container.HandleReturns(spec.Handle)
		container.GraceTimeReturns(spec.GraceTime)

		if spec.ID == "" {
			spec.ID = spec.Handle
		}

		container.IDReturns(spec.ID)

		container.HasPropertiesStub = func(props garden.Properties) bool {
			for k, v := range props {
				if value, ok := spec.Properties[k]; !ok || (ok && value != v) {
					return false
				}
			}
			return true
		}

		return container
	}

	registerTestContainer := func(container *fakes.FakeContainer) *fakes.FakeContainer {
		fakeContainers[container.Handle()] = container
		return container
	}

	BeforeEach(func() {
		fakeContainers = make(map[string]*fakes.FakeContainer)
		logger = lagertest.NewTestLogger("test")
		fakeResourcePool = new(fakes.FakeResourcePool)
		containerRepo = container_repository.New()
		fakeSystemInfo = fake_system_info.NewFakeProvider()

		snapshotsPath = ""
		maxContainers = 0

		id := 0
		fakeResourcePool.AcquireStub = func(spec garden.ContainerSpec) (linux_backend.LinuxContainerSpec, error) {
			if spec.Handle == "" {
				id = id + 1
				spec.Handle = fmt.Sprintf("handle-%d", id)
			}
			return linux_backend.LinuxContainerSpec{ContainerSpec: spec}, nil
		}

		fakeResourcePool.RestoreStub = func(snapshot io.Reader) (linux_backend.LinuxContainerSpec, error) {
			b, err := ioutil.ReadAll(snapshot)
			Expect(err).NotTo(HaveOccurred())

			return linux_backend.LinuxContainerSpec{
				ID:            string(b),
				ContainerSpec: garden.ContainerSpec{Handle: string(b)},
			}, nil
		}

		fakeContainerProvider = new(fakes.FakeContainerProvider)
		fakeContainerProvider.ProvideContainerStub = func(spec linux_backend.LinuxContainerSpec) linux_backend.Container {
			if c, ok := fakeContainers[spec.Handle]; ok {
				return c
			}

			return newTestContainer(spec)
		}
	})

	JustBeforeEach(func() {
		linuxBackend = linux_backend.New(
			logger,
			fakeResourcePool,
			containerRepo,
			fakeContainerProvider,
			fakeSystemInfo,
			snapshotsPath,
			maxContainers,
		)
	})

	Describe("Setup", func() {
		It("sets up the container pool", func() {
			err := linuxBackend.Setup()
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeResourcePool.SetupCallCount()).To(Equal(1))
		})
	})

	Describe("Start", func() {
		var tmpdir string

		BeforeEach(func() {
			var err error

			tmpdir, err = ioutil.TempDir(os.TempDir(), "garden-server-test")
			Expect(err).ToNot(HaveOccurred())

			snapshotsPath = path.Join(tmpdir, "snapshots")
		})

		It("creates the snapshots directory if it's not already there", func() {
			err := linuxBackend.Start()
			Expect(err).ToNot(HaveOccurred())

			stat, err := os.Stat(snapshotsPath)
			Expect(err).ToNot(HaveOccurred())

			Expect(stat.IsDir()).To(BeTrue())
		})

		Context("when the snapshots directory fails to be created", func() {
			BeforeEach(func() {
				tmpfile, err := ioutil.TempFile(os.TempDir(), "garden-server-test")
				Expect(err).ToNot(HaveOccurred())

				snapshotsPath = path.Join(tmpfile.Name(), "snapshots")
			})

			It("fails to start", func() {
				err := linuxBackend.Start()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when no snapshots directory is given", func() {
			It("successfully starts", func() {
				err := linuxBackend.Start()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Describe("when snapshots are present", func() {
			var snapshotsPath string

			BeforeEach(func() {
				snapshotsPath = path.Join(tmpdir, "snapshots")

				err := os.MkdirAll(snapshotsPath, 0755)
				Expect(err).ToNot(HaveOccurred())

				file, err := os.Create(path.Join(snapshotsPath, "some-id"))
				Expect(err).ToNot(HaveOccurred())

				file.Write([]byte("handle-a"))
				file.Close()

				file, err = os.Create(path.Join(snapshotsPath, "some-other-id"))
				Expect(err).ToNot(HaveOccurred())

				file.Write([]byte("handle-b"))
				file.Close()
			})

			It("restores them via the container pool", func() {
				Expect(fakeResourcePool.RestoreCallCount()).To(Equal(0))

				err := linuxBackend.Start()
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeResourcePool.RestoreCallCount()).To(Equal(2))
			})

			It("removes the snapshots", func() {
				Expect(fakeResourcePool.RestoreCallCount()).To(Equal(0))

				err := linuxBackend.Start()
				Expect(err).ToNot(HaveOccurred())

				_, err = os.Stat(path.Join(snapshotsPath, "some-id"))
				Expect(err).To(HaveOccurred())

				_, err = os.Stat(path.Join(snapshotsPath, "some-other-id"))
				Expect(err).To(HaveOccurred())
			})

			It("registers the containers", func() {
				err := linuxBackend.Start()
				Expect(err).ToNot(HaveOccurred())

				containers, err := linuxBackend.Containers(nil)
				Expect(err).ToNot(HaveOccurred())

				Expect(containers).To(HaveLen(2))
			})

			It("keeps them when pruning the container pool", func() {
				err := linuxBackend.Start()
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeResourcePool.PruneCallCount()).To(Equal(1))
				Expect(fakeResourcePool.PruneArgsForCall(0)).To(Equal(map[string]bool{
					"handle-a": true,
					"handle-b": true,
				}))
			})

			Context("when restoring the container fails", func() {
				disaster := errors.New("failed to restore")

				BeforeEach(func() {
					fakeResourcePool.RestoreReturns(linux_backend.LinuxContainerSpec{}, disaster)
				})

				It("successfully starts anyway", func() {
					err := linuxBackend.Start()
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		It("prunes the container pool", func() {
			err := linuxBackend.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeResourcePool.PruneCallCount()).To(Equal(1))
			Expect(fakeResourcePool.PruneArgsForCall(0)).To(Equal(map[string]bool{}))
		})

		Context("when pruning the container pool fails", func() {
			disaster := errors.New("failed to prune")

			BeforeEach(func() {
				fakeResourcePool.PruneReturns(disaster)
			})

			It("returns the error", func() {
				err := linuxBackend.Start()
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Stop", func() {
		var (
			container1 *fakes.FakeContainer
			container2 *fakes.FakeContainer
		)

		BeforeEach(func() {
			container1 = registerTestContainer(newTestContainer(linux_backend.LinuxContainerSpec{
				ContainerSpec: garden.ContainerSpec{Handle: "container-1"},
			}))
			containerRepo.Add(container1)
			container2 = registerTestContainer(newTestContainer(linux_backend.LinuxContainerSpec{
				ContainerSpec: garden.ContainerSpec{Handle: "container-2"},
			}))
			containerRepo.Add(container2)
		})

		Context("when no snapshot directory is passed", func() {
			It("stops succesfully without saving snapshots", func() {
				Expect(func() { linuxBackend.Stop() }).ToNot(Panic())

				Expect(container1.SnapshotCallCount()).To(Equal(0))
				Expect(container2.SnapshotCallCount()).To(Equal(0))
			})
		})

		Context("when the snapshot directory is passed", func() {
			BeforeEach(func() {
				tmpdir, err := ioutil.TempDir(os.TempDir(), "garden-server-test")
				Expect(err).ToNot(HaveOccurred())

				snapshotsPath = path.Join(tmpdir, "snapshots")
			})

			JustBeforeEach(func() {
				err := linuxBackend.Start()
				Expect(err).ToNot(HaveOccurred())
			})

			It("takes a snapshot of each container", func() {
				linuxBackend.Stop()

				Expect(container1.SnapshotCallCount()).To(Equal(1))
				Expect(container2.SnapshotCallCount()).To(Equal(1))
			})

			It("cleans up each container", func() {
				linuxBackend.Stop()

				Expect(container1.CleanupCallCount()).To(Equal(1))
				Expect(container2.CleanupCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Capacity", func() {
		It("returns the right capacity values", func() {
			fakeSystemInfo.TotalMemoryResult = 1111
			fakeSystemInfo.TotalDiskResult = 2222
			fakeResourcePool.MaxContainersReturns(42)

			capacity, err := linuxBackend.Capacity()
			Expect(err).ToNot(HaveOccurred())

			Expect(capacity.MemoryInBytes).To(Equal(uint64(1111)))
			Expect(capacity.DiskInBytes).To(Equal(uint64(2222)))
			Expect(capacity.MaxContainers).To(Equal(uint64(42)))
		})

		Context("when the max containers argument is set", func() {
			Context("and pool.MaxContainers is lower", func() {
				BeforeEach(func() {
					maxContainers = 60
					fakeResourcePool.MaxContainersReturns(40)
				})

				It("returns the pool.MaxContainers", func() {
					capacity, err := linuxBackend.Capacity()
					Expect(err).ToNot(HaveOccurred())
					Expect(capacity.MaxContainers).To(Equal(uint64(40)))
				})
			})
			Context("and pool.MaxContainers is higher", func() {
				BeforeEach(func() {
					maxContainers = 50
					fakeResourcePool.MaxContainersReturns(60)
				})

				It("returns the max containers argument", func() {
					capacity, err := linuxBackend.Capacity()
					Expect(err).ToNot(HaveOccurred())
					Expect(capacity.MaxContainers).To(Equal(uint64(50)))
				})
			})
		})

		Context("when getting memory info fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeSystemInfo.TotalMemoryError = disaster
			})

			It("returns the error", func() {
				_, err := linuxBackend.Capacity()
				Expect(err).To(Equal(disaster))
			})
		})

		Context("when getting disk info fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeSystemInfo.TotalDiskError = disaster
			})

			It("returns the error", func() {
				_, err := linuxBackend.Capacity()
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Create", func() {
		It("acquires container resources from the pool", func() {
			Expect(fakeResourcePool.AcquireCallCount()).To(Equal(0))

			spec := garden.ContainerSpec{Handle: "foo"}

			_, err := linuxBackend.Create(spec)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeResourcePool.AcquireArgsForCall(0)).To(Equal(spec))
		})

		It("starts the container", func() {
			fakeContainer := registerTestContainer(newTestContainer(
				linux_backend.LinuxContainerSpec{
					ContainerSpec: garden.ContainerSpec{Handle: "foo"},
				},
			))

			returnedContainer, err := linuxBackend.Create(garden.ContainerSpec{Handle: "foo"})
			Expect(err).ToNot(HaveOccurred())

			Expect(returnedContainer).To(Equal(fakeContainer))
			Expect(fakeContainer.StartCallCount()).To(Equal(1))
		})

		Context("when starting the container fails", func() {
			It("destroys the container", func() {
				container := registerTestContainer(newTestContainer(
					linux_backend.LinuxContainerSpec{
						ContainerSpec: garden.ContainerSpec{Handle: "disastrous"},
					},
				))

				container.StartReturns(errors.New("insufficient banana!"))

				_, err := linuxBackend.Create(garden.ContainerSpec{Handle: "disastrous"})
				Expect(err).To(HaveOccurred())
				Expect(fakeResourcePool.ReleaseCallCount()).To(Equal(1))
				Expect(fakeResourcePool.ReleaseArgsForCall(0).Handle).To(Equal("disastrous"))
			})
		})

		It("registers the container", func() {
			container, err := linuxBackend.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			foundContainer, err := linuxBackend.Lookup(container.Handle())
			Expect(err).ToNot(HaveOccurred())

			Expect(foundContainer).To(Equal(container))
		})

		Context("when creating the container fails", func() {
			disaster := errors.New("failed to create")

			BeforeEach(func() {
				fakeResourcePool.AcquireReturns(linux_backend.LinuxContainerSpec{}, disaster)
			})

			It("returns the error", func() {
				container, err := linuxBackend.Create(garden.ContainerSpec{})
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(disaster))

				Expect(container).To(BeNil())
			})
		})

		Context("when a container with the given handle already exists", func() {
			It("returns a HandleExistsError", func() {
				container, err := linuxBackend.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				_, err = linuxBackend.Create(garden.ContainerSpec{Handle: container.Handle()})
				Expect(err).To(Equal(linux_backend.HandleExistsError{container.Handle()}))
			})
		})

		Context("when starting the container fails", func() {
			disaster := errors.New("failed to start")

			BeforeEach(func() {
				container := new(fakes.FakeContainer)
				fakeContainerProvider.ProvideContainerReturns(container)
				container.StartReturns(disaster)
			})

			It("returns the error", func() {
				container, err := linuxBackend.Create(garden.ContainerSpec{})
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(disaster))

				Expect(container).To(BeNil())
			})

			It("does not register the container", func() {
				_, err := linuxBackend.Create(garden.ContainerSpec{})
				Expect(err).To(HaveOccurred())

				containers, err := linuxBackend.Containers(nil)
				Expect(err).ToNot(HaveOccurred())

				Expect(containers).To(BeEmpty())
			})
		})

		Context("when the max containers parameter is set", func() {
			BeforeEach(func() {
				maxContainers = 2
			})

			It("obeys the limit", func() {
				_, err := linuxBackend.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				_, err = linuxBackend.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				_, err = linuxBackend.Create(garden.ContainerSpec{})
				Expect(err).To(MatchError("cannot create more than 2 containers"))
			})
		})

		Context("when limits are set in the container spec", func() {
			var containerSpec garden.ContainerSpec
			var container *fakes.FakeContainer

			BeforeEach(func() {
				container = new(fakes.FakeContainer)
				fakeContainerProvider.ProvideContainerReturns(container)

				containerSpec = garden.ContainerSpec{
					Handle: "limits",
					Limits: garden.Limits{
						Disk: garden.DiskLimits{
							InodeSoft: 1,
							InodeHard: 2,
							ByteSoft:  3,
							ByteHard:  4,
							Scope:     garden.DiskLimitScopeExclusive,
						},
						Memory: garden.MemoryLimits{
							LimitInBytes: 1024,
						},
					},
				}
			})

			It("applies the limits", func() {
				_, err := linuxBackend.Create(containerSpec)
				Expect(err).ToNot(HaveOccurred())

				Expect(container.LimitCPUCallCount()).To(Equal(0))
				Expect(container.LimitBandwidthCallCount()).To(Equal(0))

				Expect(container.LimitDiskCallCount()).To(Equal(1))
				Expect(container.LimitDiskArgsForCall(0)).To(Equal(containerSpec.Limits.Disk))

				Expect(container.LimitMemoryCallCount()).To(Equal(1))
				Expect(container.LimitMemoryArgsForCall(0)).To(Equal(containerSpec.Limits.Memory))
			})

			Context("when applying limits fails", func() {
				limitErr := errors.New("failed to limit")

				BeforeEach(func() {
					container.LimitDiskReturns(limitErr)
				})

				It("returns the error", func() {
					_, err := linuxBackend.Create(containerSpec)
					Expect(err).To(MatchError(limitErr))
				})
			})
		})
	})

	Describe("Destroy", func() {
		var container *fakes.FakeContainer

		resources := linux_backend.LinuxContainerSpec{ID: "something"}

		JustBeforeEach(func() {
			container = new(fakes.FakeContainer)
			container.HandleReturns("some-handle")
			container.ResourceSpecReturns(resources)

			containerRepo.Add(container)
		})

		It("removes the given container's resoureces from the pool", func() {
			Expect(fakeResourcePool.ReleaseCallCount()).To(Equal(0))

			err := linuxBackend.Destroy("some-handle")
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeResourcePool.ReleaseCallCount()).To(Equal(1))
			Expect(fakeResourcePool.ReleaseArgsForCall(0)).To(Equal(resources))
		})

		It("unregisters the container", func() {
			err := linuxBackend.Destroy("some-handle")
			Expect(err).ToNot(HaveOccurred())

			_, err = linuxBackend.Lookup("some-handle")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(garden.ContainerNotFoundError{"some-handle"}))
		})

		Context("when the container does not exist", func() {
			It("returns ContainerNotFoundError", func() {
				err := linuxBackend.Destroy("bogus-handle")
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(garden.ContainerNotFoundError{"bogus-handle"}))
			})
		})

		Context("when destroying the container fails", func() {
			disaster := errors.New("failed to destroy")

			BeforeEach(func() {
				fakeResourcePool.ReleaseReturns(disaster)
			})

			It("returns the error", func() {
				err := linuxBackend.Destroy("some-handle")
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(disaster))
			})

			It("does not unregister the container", func() {
				err := linuxBackend.Destroy("some-handle")
				Expect(err).To(HaveOccurred())

				foundContainer, err := linuxBackend.Lookup("some-handle")
				Expect(err).ToNot(HaveOccurred())
				Expect(foundContainer).To(Equal(container))
			})
		})
	})

	Describe("BulkInfo", func() {
		newContainer := func(handle string) *fakes.FakeContainer {
			fakeContainer := &fakes.FakeContainer{}
			fakeContainer.HandleReturns(handle)
			fakeContainer.InfoReturns(
				garden.ContainerInfo{
					HostIP: "hostip for " + handle,
				},
				nil,
			)
			return fakeContainer
		}

		container1 := newContainer("handle1")
		container2 := newContainer("handle2")
		handles := []string{"handle1", "handle2"}

		BeforeEach(func() {
			containerRepo.Add(container1)
			containerRepo.Add(container2)
		})

		It("returns info about containers", func() {
			bulkInfo, err := linuxBackend.BulkInfo(handles)
			Expect(err).ToNot(HaveOccurred())

			Expect(bulkInfo).To(Equal(map[string]garden.ContainerInfoEntry{
				container1.Handle(): garden.ContainerInfoEntry{
					Info: garden.ContainerInfo{
						HostIP: "hostip for handle1",
					},
				},
				container2.Handle(): garden.ContainerInfoEntry{
					Info: garden.ContainerInfo{
						HostIP: "hostip for handle2",
					},
				},
			}))
		})

		Context("when not all of the handles in the system are requested", func() {
			handles := []string{"handle2"}

			It("returns info about the specified containers", func() {
				bulkInfo, err := linuxBackend.BulkInfo(handles)
				Expect(err).ToNot(HaveOccurred())

				Expect(bulkInfo).To(Equal(map[string]garden.ContainerInfoEntry{
					container2.Handle(): garden.ContainerInfoEntry{
						Info: garden.ContainerInfo{
							HostIP: "hostip for handle2",
						},
					},
				}))
			})
		})

		Context("when getting one of the infos for a container fails", func() {
			handles := []string{"handle1", "handle2"}

			BeforeEach(func() {
				container2.InfoReturns(garden.ContainerInfo{}, errors.New("Oh no!"))
			})

			It("returns the err for the failed container", func() {
				bulkInfo, err := linuxBackend.BulkInfo(handles)
				Expect(err).ToNot(HaveOccurred())

				Expect(bulkInfo).To(Equal(map[string]garden.ContainerInfoEntry{
					container1.Handle(): garden.ContainerInfoEntry{
						Info: garden.ContainerInfo{
							HostIP: "hostip for handle1",
						},
					},
					container2.Handle(): garden.ContainerInfoEntry{
						Err: garden.NewError("Oh no!"),
					},
				}))
			})
		})
	})

	Describe("BulkMetrics", func() {
		newContainer := func(n uint64) *fakes.FakeContainer {
			fakeContainer := &fakes.FakeContainer{}
			fakeContainer.HandleReturns(fmt.Sprintf("handle%d", n))
			fakeContainer.MetricsReturns(
				garden.Metrics{
					DiskStat: garden.ContainerDiskStat{
						TotalInodesUsed: n,
					},
				},
				nil,
			)
			return fakeContainer
		}

		container1 := newContainer(1)
		container2 := newContainer(2)
		handles := []string{"handle1", "handle2"}

		BeforeEach(func() {
			containerRepo.Add(container1)
			containerRepo.Add(container2)
		})

		It("returns info about containers", func() {
			bulkMetrics, err := linuxBackend.BulkMetrics(handles)
			Expect(err).ToNot(HaveOccurred())

			Expect(bulkMetrics).To(Equal(map[string]garden.ContainerMetricsEntry{
				container1.Handle(): garden.ContainerMetricsEntry{
					Metrics: garden.Metrics{
						DiskStat: garden.ContainerDiskStat{
							TotalInodesUsed: 1,
						},
					},
				},
				container2.Handle(): garden.ContainerMetricsEntry{
					Metrics: garden.Metrics{
						DiskStat: garden.ContainerDiskStat{
							TotalInodesUsed: 2,
						},
					},
				},
			}))
		})

		Context("when not all of the handles in the system are requested", func() {
			handles := []string{"handle2"}

			It("returns info about the specified containers", func() {
				bulkMetrics, err := linuxBackend.BulkMetrics(handles)
				Expect(err).ToNot(HaveOccurred())

				Expect(bulkMetrics).To(Equal(map[string]garden.ContainerMetricsEntry{
					container2.Handle(): garden.ContainerMetricsEntry{
						Metrics: garden.Metrics{
							DiskStat: garden.ContainerDiskStat{
								TotalInodesUsed: 2,
							},
						},
					},
				}))
			})
		})

		Context("when getting one of the infos for a container fails", func() {
			handles := []string{"handle1", "handle2"}

			BeforeEach(func() {
				container2.MetricsReturns(garden.Metrics{}, errors.New("Oh no!"))
			})

			It("returns the err for the failed container", func() {
				bulkMetrics, err := linuxBackend.BulkMetrics(handles)
				Expect(err).ToNot(HaveOccurred())

				Expect(bulkMetrics).To(Equal(map[string]garden.ContainerMetricsEntry{
					container1.Handle(): garden.ContainerMetricsEntry{
						Metrics: garden.Metrics{
							DiskStat: garden.ContainerDiskStat{
								TotalInodesUsed: 1,
							},
						},
					},
					container2.Handle(): garden.ContainerMetricsEntry{
						Err: garden.NewError("Oh no!"),
					},
				}))
			})
		})
	})

	Describe("Lookup", func() {
		It("returns the container", func() {
			container, err := linuxBackend.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			foundContainer, err := linuxBackend.Lookup(container.Handle())
			Expect(err).ToNot(HaveOccurred())

			Expect(foundContainer).To(Equal(container))
		})

		Context("when the handle is not found", func() {
			It("returns ContainerNotFoundError", func() {
				foundContainer, err := linuxBackend.Lookup("bogus-handle")
				Expect(err).To(HaveOccurred())
				Expect(foundContainer).To(BeNil())

				Expect(err).To(Equal(garden.ContainerNotFoundError{"bogus-handle"}))
			})
		})
	})

	Describe("Containers", func() {
		It("returns a list of all existing containers", func() {
			container1, err := linuxBackend.Create(garden.ContainerSpec{Handle: "container-1"})
			Expect(err).ToNot(HaveOccurred())

			container2, err := linuxBackend.Create(garden.ContainerSpec{Handle: "container-2"})
			Expect(err).ToNot(HaveOccurred())

			containers, err := linuxBackend.Containers(nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(containers).To(ContainElement(container1))
			Expect(containers).To(ContainElement(container2))
		})

		Context("when given properties to filter by", func() {
			It("returns only containers with matching properties", func() {
				container1, err := linuxBackend.Create(garden.ContainerSpec{
					Handle:     "container-1",
					Properties: garden.Properties{"a": "b"},
				})
				Expect(err).ToNot(HaveOccurred())

				container2, err := linuxBackend.Create(garden.ContainerSpec{
					Handle:     "container-2",
					Properties: garden.Properties{"a": "b"},
				})
				Expect(err).ToNot(HaveOccurred())

				container3, err := linuxBackend.Create(garden.ContainerSpec{
					Handle:     "container-3",
					Properties: garden.Properties{"a": "b", "c": "d", "e": "f"},
				})
				Expect(err).ToNot(HaveOccurred())

				containers, err := linuxBackend.Containers(
					garden.Properties{"a": "b", "e": "f"},
				)
				Expect(err).ToNot(HaveOccurred())

				Expect(containers).ToNot(ContainElement(container1))
				Expect(containers).ToNot(ContainElement(container2))
				Expect(containers).To(ContainElement(container3))
			})
		})
	})

	Describe("GraceTime", func() {
		It("returns the container's grace time", func() {
			container, err := linuxBackend.Create(garden.ContainerSpec{
				GraceTime: time.Second,
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(linuxBackend.GraceTime(container)).To(Equal(time.Second))
		})
	})
})
