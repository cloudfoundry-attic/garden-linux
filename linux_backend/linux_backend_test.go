package linux_backend_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_pool/fake_container_pool"
	"github.com/cloudfoundry-incubator/garden-linux/container_repository"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/old/system_info/fake_system_info"
)

var _ = Describe("LinuxBackend", func() {
	var logger *lagertest.TestLogger

	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var fakeSystemInfo *fake_system_info.FakeProvider
	var containerRepo linux_backend.ContainerRepository
	var linuxBackend *linux_backend.LinuxBackend
	var snapshotsPath string

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		fakeContainerPool = fake_container_pool.New()
		containerRepo = container_repository.New()
		fakeSystemInfo = fake_system_info.NewFakeProvider()

		snapshotsPath = ""
	})

	JustBeforeEach(func() {
		linuxBackend = linux_backend.New(
			logger,
			fakeContainerPool,
			containerRepo,
			fakeSystemInfo,
			snapshotsPath,
		)
	})

	Describe("Setup", func() {
		It("sets up the container pool", func() {
			err := linuxBackend.Setup()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeContainerPool.DidSetup).Should(BeTrue())
		})
	})

	Describe("Start", func() {
		var tmpdir string

		BeforeEach(func() {
			var err error

			tmpdir, err = ioutil.TempDir(os.TempDir(), "garden-server-test")
			Ω(err).ShouldNot(HaveOccurred())

			snapshotsPath = path.Join(tmpdir, "snapshots")
		})

		It("creates the snapshots directory if it's not already there", func() {
			err := linuxBackend.Start()
			Ω(err).ShouldNot(HaveOccurred())

			stat, err := os.Stat(snapshotsPath)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(stat.IsDir()).Should(BeTrue())
		})

		Context("when the snapshots directory fails to be created", func() {
			BeforeEach(func() {
				tmpfile, err := ioutil.TempFile(os.TempDir(), "garden-server-test")
				Ω(err).ShouldNot(HaveOccurred())

				snapshotsPath = path.Join(tmpfile.Name(), "snapshots")
			})

			It("fails to start", func() {
				err := linuxBackend.Start()
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when no snapshots directory is given", func() {
			It("successfully starts", func() {
				err := linuxBackend.Start()
				Ω(err).ShouldNot(HaveOccurred())
			})
		})

		Describe("when snapshots are present", func() {
			var snapshotsPath string

			BeforeEach(func() {
				snapshotsPath = path.Join(tmpdir, "snapshots")

				err := os.MkdirAll(snapshotsPath, 0755)
				Ω(err).ShouldNot(HaveOccurred())

				file, err := os.Create(path.Join(snapshotsPath, "some-id"))
				Ω(err).ShouldNot(HaveOccurred())

				file.Write([]byte("handle-a"))
				file.Close()

				file, err = os.Create(path.Join(snapshotsPath, "some-other-id"))
				Ω(err).ShouldNot(HaveOccurred())

				file.Write([]byte("handle-b"))
				file.Close()
			})

			It("restores them via the container pool", func() {
				Ω(fakeContainerPool.RestoredSnapshots).Should(BeEmpty())

				err := linuxBackend.Start()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeContainerPool.RestoredSnapshots).Should(HaveLen(2))
			})

			It("removes the snapshots", func() {
				Ω(fakeContainerPool.RestoredSnapshots).Should(BeEmpty())

				err := linuxBackend.Start()
				Ω(err).ShouldNot(HaveOccurred())

				_, err = os.Stat(path.Join(snapshotsPath, "some-id"))
				Ω(err).Should(HaveOccurred())

				_, err = os.Stat(path.Join(snapshotsPath, "some-other-id"))
				Ω(err).Should(HaveOccurred())
			})

			It("registers the containers", func() {
				err := linuxBackend.Start()
				Ω(err).ShouldNot(HaveOccurred())

				containers, err := linuxBackend.Containers(nil)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(containers).Should(HaveLen(2))
			})

			It("keeps them when pruning the container pool", func() {
				err := linuxBackend.Start()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeContainerPool.Pruned).Should(BeTrue())
				Ω(fakeContainerPool.KeptContainers).Should(Equal(map[string]bool{
					"handle-a": true,
					"handle-b": true,
				}))
			})

			Context("when restoring the container fails", func() {
				disaster := errors.New("failed to restore")

				BeforeEach(func() {
					fakeContainerPool.RestoreError = disaster
				})

				It("successfully starts anyway", func() {
					err := linuxBackend.Start()
					Ω(err).ShouldNot(HaveOccurred())
				})
			})
		})

		It("prunes the container pool", func() {
			err := linuxBackend.Start()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeContainerPool.Pruned).Should(BeTrue())
			Ω(fakeContainerPool.KeptContainers).Should(Equal(map[string]bool{}))
		})

		Context("when pruning the container pool fails", func() {
			disaster := errors.New("failed to prune")

			BeforeEach(func() {
				fakeContainerPool.PruneError = disaster
			})

			It("returns the error", func() {
				err := linuxBackend.Start()
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Stop", func() {
		var (
			container1 *fake_container_pool.FakeContainer
			container2 *fake_container_pool.FakeContainer
		)

		BeforeEach(func() {
			container1 = fake_container_pool.NewFakeContainer(
				garden.ContainerSpec{
					Handle: "some-handle",
				},
			)

			container2 = fake_container_pool.NewFakeContainer(
				garden.ContainerSpec{
					Handle: "some-other-handle",
				},
			)

			containerRepo.Add(container1)
			containerRepo.Add(container2)
		})

		Context("when no snapshot directory is passed", func() {
			It("stops succesfully without saving snapshots", func() {
				Ω(func() { linuxBackend.Stop() }).ShouldNot(Panic())

				Ω(container1.SavedSnapshots).Should(HaveLen(0))
				Ω(container2.SavedSnapshots).Should(HaveLen(0))
			})
		})

		Context("when the snapshot directory is passed", func() {
			BeforeEach(func() {
				tmpdir, err := ioutil.TempDir(os.TempDir(), "garden-server-test")
				Ω(err).ShouldNot(HaveOccurred())

				snapshotsPath = path.Join(tmpdir, "snapshots")
			})

			JustBeforeEach(func() {
				err := linuxBackend.Start()
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("takes a snapshot of each container", func() {
				linuxBackend.Stop()

				Ω(container1.SavedSnapshots).Should(HaveLen(1))
				Ω(container2.SavedSnapshots).Should(HaveLen(1))
			})

			It("cleans up each container", func() {
				linuxBackend.Stop()

				Ω(container1.CleanedUp).Should(BeTrue())
				Ω(container2.CleanedUp).Should(BeTrue())
			})
		})
	})

	Describe("Capacity", func() {
		It("returns the right capacity values", func() {
			fakeSystemInfo.TotalMemoryResult = 1111
			fakeSystemInfo.TotalDiskResult = 2222
			fakeContainerPool.MaxContainersValue = 42

			capacity, err := linuxBackend.Capacity()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(capacity.MemoryInBytes).Should(Equal(uint64(1111)))
			Ω(capacity.DiskInBytes).Should(Equal(uint64(2222)))
			Ω(capacity.MaxContainers).Should(Equal(uint64(42)))
		})

		Context("when getting memory info fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeSystemInfo.TotalMemoryError = disaster
			})

			It("returns the error", func() {
				_, err := linuxBackend.Capacity()
				Ω(err).Should(Equal(disaster))
			})
		})

		Context("when getting disk info fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeSystemInfo.TotalDiskError = disaster
			})

			It("returns the error", func() {
				_, err := linuxBackend.Capacity()
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Create", func() {
		It("creates a container from the pool", func() {
			Ω(fakeContainerPool.CreatedContainers).Should(BeEmpty())

			container, err := linuxBackend.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeContainerPool.CreatedContainers).Should(ContainElement(container))
		})

		It("starts the container", func() {
			container, err := linuxBackend.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())
			Ω(container.(*fake_container_pool.FakeContainer).Started).Should(BeTrue())
		})

		Context("when starting the container fails", func() {
			It("destroys the container", func() {
				var setupContainer *fake_container_pool.FakeContainer
				fakeContainerPool.ContainerSetup = func(c *fake_container_pool.FakeContainer) {
					c.StartError = errors.New("insufficient banana")
					setupContainer = c
				}

				_, err := linuxBackend.Create(garden.ContainerSpec{})
				Ω(err).Should(HaveOccurred())
				Ω(fakeContainerPool.DestroyedContainers).Should(ContainElement(setupContainer))
			})
		})

		It("registers the container", func() {
			container, err := linuxBackend.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			foundContainer, err := linuxBackend.Lookup(container.Handle())
			Ω(err).ShouldNot(HaveOccurred())

			Ω(foundContainer).Should(Equal(container))
		})

		Context("when creating the container fails", func() {
			disaster := errors.New("failed to create")

			BeforeEach(func() {
				fakeContainerPool.CreateError = disaster
			})

			It("returns the error", func() {
				container, err := linuxBackend.Create(garden.ContainerSpec{})
				Ω(err).Should(HaveOccurred())
				Ω(err).Should(Equal(disaster))

				Ω(container).Should(BeNil())
			})
		})

		Context("when a container with the given handle already exists", func() {
			It("returns a HandleExistsError", func() {
				container, err := linuxBackend.Create(garden.ContainerSpec{})
				Ω(err).ShouldNot(HaveOccurred())

				_, err = linuxBackend.Create(garden.ContainerSpec{Handle: container.Handle()})
				Ω(err).Should(Equal(linux_backend.HandleExistsError{container.Handle()}))
			})
		})

		Context("when starting the container fails", func() {
			disaster := errors.New("failed to start")

			BeforeEach(func() {
				fakeContainerPool.ContainerSetup = func(c *fake_container_pool.FakeContainer) {
					c.StartError = disaster
				}
			})

			It("returns the error", func() {
				container, err := linuxBackend.Create(garden.ContainerSpec{})
				Ω(err).Should(HaveOccurred())
				Ω(err).Should(Equal(disaster))

				Ω(container).Should(BeNil())
			})

			It("does not register the container", func() {
				_, err := linuxBackend.Create(garden.ContainerSpec{})
				Ω(err).Should(HaveOccurred())

				containers, err := linuxBackend.Containers(nil)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(containers).Should(BeEmpty())
			})
		})
	})

	Describe("Destroy", func() {
		var container *fake_container_pool.FakeContainer

		JustBeforeEach(func() {
			container = fake_container_pool.NewFakeContainer(garden.ContainerSpec{Handle: "some-handle"})
			containerRepo.Add(container)
		})

		It("removes the given container from the pool", func() {
			Ω(fakeContainerPool.DestroyedContainers).Should(BeEmpty())

			err := linuxBackend.Destroy("some-handle")
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeContainerPool.DestroyedContainers).Should(ContainElement(container))
		})

		It("unregisters the container", func() {
			err := linuxBackend.Destroy("some-handle")
			Ω(err).ShouldNot(HaveOccurred())

			_, err = linuxBackend.Lookup("some-handle")
			Ω(err).Should(HaveOccurred())
			Ω(err).Should(MatchError(garden.ContainerNotFoundError{"some-handle"}))
		})

		Context("when the container does not exist", func() {
			It("returns ContainerNotFoundError", func() {
				err := linuxBackend.Destroy("bogus-handle")
				Ω(err).Should(HaveOccurred())
				Ω(err).Should(Equal(garden.ContainerNotFoundError{"bogus-handle"}))
			})
		})

		Context("when destroying the container fails", func() {
			disaster := errors.New("failed to destroy")

			BeforeEach(func() {
				fakeContainerPool.DestroyError = disaster
			})

			It("returns the error", func() {
				err := linuxBackend.Destroy("some-handle")
				Ω(err).Should(HaveOccurred())
				Ω(err).Should(Equal(disaster))
			})

			It("does not unregister the container", func() {
				err := linuxBackend.Destroy("some-handle")
				Ω(err).Should(HaveOccurred())

				foundContainer, err := linuxBackend.Lookup("some-handle")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(foundContainer).Should(Equal(container))
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
			Ω(err).ShouldNot(HaveOccurred())

			Ω(bulkInfo).Should(Equal(map[string]garden.ContainerInfoEntry{
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
				Ω(err).ShouldNot(HaveOccurred())

				Ω(bulkInfo).Should(Equal(map[string]garden.ContainerInfoEntry{
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
				Ω(err).ShouldNot(HaveOccurred())

				Ω(bulkInfo).Should(Equal(map[string]garden.ContainerInfoEntry{
					container1.Handle(): garden.ContainerInfoEntry{
						Info: garden.ContainerInfo{
							HostIP: "hostip for handle1",
						},
					},
					container2.Handle(): garden.ContainerInfoEntry{
						Err: errors.New("Oh no!"),
					},
				}))
			})
		})
	})

	Describe("Lookup", func() {
		It("returns the container", func() {
			container, err := linuxBackend.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			foundContainer, err := linuxBackend.Lookup(container.Handle())
			Ω(err).ShouldNot(HaveOccurred())

			Ω(foundContainer).Should(Equal(container))
		})

		Context("when the handle is not found", func() {
			It("returns ContainerNotFoundError", func() {
				foundContainer, err := linuxBackend.Lookup("bogus-handle")
				Ω(err).Should(HaveOccurred())
				Ω(foundContainer).Should(BeNil())

				Ω(err).Should(Equal(garden.ContainerNotFoundError{"bogus-handle"}))
			})
		})
	})

	Describe("Containers", func() {
		It("returns a list of all existing containers", func() {
			container1, err := linuxBackend.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			container2, err := linuxBackend.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			containers, err := linuxBackend.Containers(nil)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(containers).Should(ContainElement(container1))
			Ω(containers).Should(ContainElement(container2))
		})

		Context("when given properties to filter by", func() {
			It("returns only containers with matching properties", func() {
				container1, err := linuxBackend.Create(garden.ContainerSpec{
					Properties: garden.Properties{"a": "b"},
				})
				Ω(err).ShouldNot(HaveOccurred())

				container2, err := linuxBackend.Create(garden.ContainerSpec{
					Properties: garden.Properties{"a": "b"},
				})
				Ω(err).ShouldNot(HaveOccurred())

				container3, err := linuxBackend.Create(garden.ContainerSpec{
					Properties: garden.Properties{"a": "b", "c": "d", "e": "f"},
				})
				Ω(err).ShouldNot(HaveOccurred())

				containers, err := linuxBackend.Containers(
					garden.Properties{"a": "b", "e": "f"},
				)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(containers).ShouldNot(ContainElement(container1))
				Ω(containers).ShouldNot(ContainElement(container2))
				Ω(containers).Should(ContainElement(container3))
			})
		})
	})

	Describe("GraceTime", func() {
		It("returns the container's grace time", func() {
			container, err := linuxBackend.Create(garden.ContainerSpec{
				GraceTime: time.Second,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(linuxBackend.GraceTime(container)).Should(Equal(time.Second))
		})
	})
})
