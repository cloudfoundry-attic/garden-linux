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

	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/container_pool/fake_container_pool"
	"github.com/cloudfoundry-incubator/garden-linux/system_info/fake_system_info"
	"github.com/cloudfoundry-incubator/garden/warden"
)

var logger *lagertest.TestLogger

var _ = BeforeEach(func() {
	logger = lagertest.NewTestLogger("test")
})

var _ = Describe("Setup", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var fakeSystemInfo *fake_system_info.FakeProvider
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo = fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(lagertest.NewTestLogger("test"), fakeContainerPool, fakeSystemInfo, "")
	})

	It("sets up the container pool", func() {
		err := linuxBackend.Setup()
		Ω(err).ShouldNot(HaveOccurred())

		Ω(fakeContainerPool.DidSetup).Should(BeTrue())
	})
})

var _ = Describe("Start", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var fakeSystemInfo *fake_system_info.FakeProvider

	var tmpdir string

	BeforeEach(func() {
		var err error

		tmpdir, err = ioutil.TempDir(os.TempDir(), "warden-server-test")
		Ω(err).ShouldNot(HaveOccurred())

		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo = fake_system_info.NewFakeProvider()
	})

	It("creates the snapshots directory if it's not already there", func() {
		snapshotsPath := path.Join(tmpdir, "snapshots")

		linuxBackend := linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, snapshotsPath)

		err := linuxBackend.Start()
		Ω(err).ShouldNot(HaveOccurred())

		stat, err := os.Stat(snapshotsPath)
		Ω(err).ShouldNot(HaveOccurred())

		Ω(stat.IsDir()).Should(BeTrue())
	})

	Context("when the snapshots directory fails to be created", func() {
		It("fails to start", func() {
			tmpfile, err := ioutil.TempFile(os.TempDir(), "warden-server-test")
			Ω(err).ShouldNot(HaveOccurred())

			linuxBackend := linux_backend.New(
				logger,
				fakeContainerPool,
				fakeSystemInfo,
				// weird scenario: /foo/X/snapshots with X being a file
				path.Join(tmpfile.Name(), "snapshots"),
			)

			err = linuxBackend.Start()
			Ω(err).Should(HaveOccurred())
		})
	})

	Context("when no snapshots directory is given", func() {
		It("successfully starts", func() {
			linuxBackend := linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, "")

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
			linuxBackend := linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, snapshotsPath)

			Ω(fakeContainerPool.RestoredSnapshots).Should(BeEmpty())

			err := linuxBackend.Start()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeContainerPool.RestoredSnapshots).Should(HaveLen(2))
		})

		It("removes the snapshots", func() {
			linuxBackend := linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, snapshotsPath)

			Ω(fakeContainerPool.RestoredSnapshots).Should(BeEmpty())

			err := linuxBackend.Start()
			Ω(err).ShouldNot(HaveOccurred())

			_, err = os.Stat(path.Join(snapshotsPath, "some-id"))
			Ω(err).Should(HaveOccurred())

			_, err = os.Stat(path.Join(snapshotsPath, "some-other-id"))
			Ω(err).Should(HaveOccurred())
		})

		It("registers the containers", func() {
			linuxBackend := linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, snapshotsPath)

			err := linuxBackend.Start()
			Ω(err).ShouldNot(HaveOccurred())

			containers, err := linuxBackend.Containers(nil)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(containers).Should(HaveLen(2))
		})

		It("keeps them when pruning the container pool", func() {
			linuxBackend := linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, snapshotsPath)

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
				linuxBackend := linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, snapshotsPath)

				err := linuxBackend.Start()
				Ω(err).ShouldNot(HaveOccurred())
			})
		})
	})

	It("prunes the container pool", func() {
		linuxBackend := linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, "")

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
			linuxBackend := linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, "")

			err := linuxBackend.Start()
			Ω(err).Should(Equal(disaster))
		})
	})
})

var _ = Describe("Stop", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var fakeSystemInfo *fake_system_info.FakeProvider
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		tmpdir, err := ioutil.TempDir(os.TempDir(), "warden-server-test")
		Ω(err).ShouldNot(HaveOccurred())

		fakeContainerPool = fake_container_pool.New()
		linuxBackend = linux_backend.New(
			logger,
			fakeContainerPool,
			fakeSystemInfo,
			path.Join(tmpdir, "snapshots"),
		)

		err = linuxBackend.Start()
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("takes a snapshot of each container", func() {
		container1, err := linuxBackend.Create(warden.ContainerSpec{Env: []string{"env1=env1Value", "env2=env2Value"}, Handle: "some-handle"})
		Ω(err).ShouldNot(HaveOccurred())

		container2, err := linuxBackend.Create(warden.ContainerSpec{Handle: "some-other-handle"})
		Ω(err).ShouldNot(HaveOccurred())

		linuxBackend.Stop()

		fakeContainer1 := container1.(*fake_container_pool.FakeContainer)
		fakeContainer2 := container2.(*fake_container_pool.FakeContainer)
		Ω(fakeContainer1.SavedSnapshots).Should(HaveLen(1))
		Ω(fakeContainer2.SavedSnapshots).Should(HaveLen(1))
	})

	It("cleans up each container", func() {
		container1, err := linuxBackend.Create(warden.ContainerSpec{Handle: "some-handle"})
		Ω(err).ShouldNot(HaveOccurred())

		container2, err := linuxBackend.Create(warden.ContainerSpec{Handle: "some-other-handle"})
		Ω(err).ShouldNot(HaveOccurred())

		linuxBackend.Stop()

		fakeContainer1 := container1.(*fake_container_pool.FakeContainer)
		fakeContainer2 := container2.(*fake_container_pool.FakeContainer)
		Ω(fakeContainer1.CleanedUp).Should(BeTrue())
		Ω(fakeContainer2.CleanedUp).Should(BeTrue())
	})
})

var _ = Describe("Capacity", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var fakeSystemInfo *fake_system_info.FakeProvider
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo = fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, "")
	})

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

var _ = Describe("Create", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo := fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, "")
	})

	It("creates a container from the pool", func() {
		Ω(fakeContainerPool.CreatedContainers).Should(BeEmpty())

		container, err := linuxBackend.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		Ω(fakeContainerPool.CreatedContainers).Should(ContainElement(container))
	})

	It("starts the container", func() {
		container, err := linuxBackend.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(container.(*fake_container_pool.FakeContainer).Started).Should(BeTrue())
	})

	It("registers the container", func() {
		container, err := linuxBackend.Create(warden.ContainerSpec{})
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
			container, err := linuxBackend.Create(warden.ContainerSpec{})
			Ω(err).Should(HaveOccurred())
			Ω(err).Should(Equal(disaster))

			Ω(container).Should(BeNil())
		})
	})

	Context("when a container with the given handle already exists", func() {
		It("returns a HandleExistsError", func() {
			container, err := linuxBackend.Create(warden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			_, err = linuxBackend.Create(warden.ContainerSpec{Handle: container.Handle()})
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
			container, err := linuxBackend.Create(warden.ContainerSpec{})
			Ω(err).Should(HaveOccurred())
			Ω(err).Should(Equal(disaster))

			Ω(container).Should(BeNil())
		})

		It("does not register the container", func() {
			_, err := linuxBackend.Create(warden.ContainerSpec{})
			Ω(err).Should(HaveOccurred())

			containers, err := linuxBackend.Containers(nil)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(containers).Should(BeEmpty())
		})
	})
})

var _ = Describe("Destroy", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var linuxBackend *linux_backend.LinuxBackend

	var container warden.Container

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo := fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, "")

		newContainer, err := linuxBackend.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		container = newContainer
	})

	It("removes the given container from the pool", func() {
		Ω(fakeContainerPool.DestroyedContainers).Should(BeEmpty())

		err := linuxBackend.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		Ω(fakeContainerPool.DestroyedContainers).Should(ContainElement(container))
	})

	It("unregisters the container", func() {
		err := linuxBackend.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		_, err = linuxBackend.Lookup(container.Handle())
		Ω(err).Should(HaveOccurred())
		Ω(err).Should(Equal(linux_backend.UnknownHandleError{container.Handle()}))
	})

	Context("when the container does not exist", func() {
		It("returns UnknownHandleError", func() {
			err := linuxBackend.Destroy("bogus-handle")
			Ω(err).Should(HaveOccurred())
			Ω(err).Should(Equal(linux_backend.UnknownHandleError{"bogus-handle"}))
		})
	})

	Context("when destroying the container fails", func() {
		disaster := errors.New("failed to destroy")

		BeforeEach(func() {
			fakeContainerPool.DestroyError = disaster
		})

		It("returns the error", func() {
			err := linuxBackend.Destroy(container.Handle())
			Ω(err).Should(HaveOccurred())
			Ω(err).Should(Equal(disaster))
		})

		It("does not unregister the container", func() {
			err := linuxBackend.Destroy(container.Handle())
			Ω(err).Should(HaveOccurred())

			foundContainer, err := linuxBackend.Lookup(container.Handle())
			Ω(err).ShouldNot(HaveOccurred())
			Ω(foundContainer).Should(Equal(container))
		})
	})
})

var _ = Describe("Lookup", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo := fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, "")
	})

	It("returns the container", func() {
		container, err := linuxBackend.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		foundContainer, err := linuxBackend.Lookup(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		Ω(foundContainer).Should(Equal(container))
	})

	Context("when the handle is not found", func() {
		It("returns UnknownHandleError", func() {
			foundContainer, err := linuxBackend.Lookup("bogus-handle")
			Ω(err).Should(HaveOccurred())
			Ω(foundContainer).Should(BeNil())

			Ω(err).Should(Equal(linux_backend.UnknownHandleError{"bogus-handle"}))
		})
	})
})

var _ = Describe("Containers", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo := fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, "")
	})

	It("returns a list of all existing containers", func() {
		container1, err := linuxBackend.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		container2, err := linuxBackend.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		containers, err := linuxBackend.Containers(nil)
		Ω(err).ShouldNot(HaveOccurred())

		Ω(containers).Should(ContainElement(container1))
		Ω(containers).Should(ContainElement(container2))
	})

	Context("when given properties to filter by", func() {
		It("returns only containers with matching properties", func() {
			container1, err := linuxBackend.Create(warden.ContainerSpec{
				Properties: warden.Properties{"a": "b"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			container2, err := linuxBackend.Create(warden.ContainerSpec{
				Properties: warden.Properties{"a": "b"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			container3, err := linuxBackend.Create(warden.ContainerSpec{
				Properties: warden.Properties{"a": "b", "c": "d", "e": "f"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			containers, err := linuxBackend.Containers(
				warden.Properties{"a": "b", "e": "f"},
			)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(containers).ShouldNot(ContainElement(container1))
			Ω(containers).ShouldNot(ContainElement(container2))
			Ω(containers).Should(ContainElement(container3))
		})
	})
})

var _ = Describe("GraceTime", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo := fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(logger, fakeContainerPool, fakeSystemInfo, "")
	})

	It("returns the container's grace time", func() {
		container, err := linuxBackend.Create(warden.ContainerSpec{
			GraceTime: time.Second,
		})
		Ω(err).ShouldNot(HaveOccurred())

		Ω(linuxBackend.GraceTime(container)).Should(Equal(time.Second))
	})
})
