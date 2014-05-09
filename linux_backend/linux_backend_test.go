package linux_backend_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/garden/warden/fake_backend"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/fake_container_pool"
	"github.com/cloudfoundry-incubator/warden-linux/system_info/fake_system_info"
)

var _ = Describe("Setup", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var fakeSystemInfo *fake_system_info.FakeProvider
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo = fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(fakeContainerPool, fakeSystemInfo, "")
	})

	It("sets up the container pool", func() {
		err := linuxBackend.Setup()
		Expect(err).ToNot(HaveOccurred())

		Expect(fakeContainerPool.DidSetup).To(BeTrue())
	})
})

var _ = Describe("Start", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var fakeSystemInfo *fake_system_info.FakeProvider

	var tmpdir string

	BeforeEach(func() {
		var err error

		tmpdir, err = ioutil.TempDir(os.TempDir(), "warden-server-test")
		Expect(err).ToNot(HaveOccurred())

		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo = fake_system_info.NewFakeProvider()
	})

	It("creates the snapshots directory if it's not already there", func() {
		snapshotsPath := path.Join(tmpdir, "snapshots")

		linuxBackend := linux_backend.New(fakeContainerPool, fakeSystemInfo, snapshotsPath)

		err := linuxBackend.Start()
		Expect(err).ToNot(HaveOccurred())

		stat, err := os.Stat(snapshotsPath)
		Expect(err).ToNot(HaveOccurred())

		Expect(stat.IsDir()).To(BeTrue())
	})

	Context("when the snapshots directory fails to be created", func() {
		It("fails to start", func() {
			tmpfile, err := ioutil.TempFile(os.TempDir(), "warden-server-test")
			Expect(err).ToNot(HaveOccurred())

			linuxBackend := linux_backend.New(
				fakeContainerPool,
				fakeSystemInfo,
				// weird scenario: /foo/X/snapshots with X being a file
				path.Join(tmpfile.Name(), "snapshots"),
			)

			err = linuxBackend.Start()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when no snapshots directory is given", func() {
		It("successfully starts", func() {
			linuxBackend := linux_backend.New(fakeContainerPool, fakeSystemInfo, "")

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
			linuxBackend := linux_backend.New(fakeContainerPool, fakeSystemInfo, snapshotsPath)

			Expect(fakeContainerPool.RestoredSnapshots).To(BeEmpty())

			err := linuxBackend.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeContainerPool.RestoredSnapshots).To(HaveLen(2))
		})

		It("removes the snapshots", func() {
			linuxBackend := linux_backend.New(fakeContainerPool, fakeSystemInfo, snapshotsPath)

			Expect(fakeContainerPool.RestoredSnapshots).To(BeEmpty())

			err := linuxBackend.Start()
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Stat(path.Join(snapshotsPath, "some-id"))
			Expect(err).To(HaveOccurred())

			_, err = os.Stat(path.Join(snapshotsPath, "some-other-id"))
			Expect(err).To(HaveOccurred())
		})

		It("registers the containers", func() {
			linuxBackend := linux_backend.New(fakeContainerPool, fakeSystemInfo, snapshotsPath)

			err := linuxBackend.Start()
			Expect(err).ToNot(HaveOccurred())

			containers, err := linuxBackend.Containers(nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(containers).To(HaveLen(2))
		})

		It("keeps them when pruning the container pool", func() {
			linuxBackend := linux_backend.New(fakeContainerPool, fakeSystemInfo, snapshotsPath)

			err := linuxBackend.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeContainerPool.Pruned).To(BeTrue())
			Expect(fakeContainerPool.KeptContainers).To(Equal(map[string]bool{
				"handle-a": true,
				"handle-b": true,
			}))
		})

		Context("when restoring the container fails", func() {
			disaster := errors.New("failed to restore")

			BeforeEach(func() {
				fakeContainerPool.RestoreError = disaster
			})

			It("returns the error", func() {
				linuxBackend := linux_backend.New(fakeContainerPool, fakeSystemInfo, snapshotsPath)

				err := linuxBackend.Start()
				Expect(err).To(Equal(disaster))
			})
		})
	})

	It("prunes the container pool", func() {
		linuxBackend := linux_backend.New(fakeContainerPool, fakeSystemInfo, "")

		err := linuxBackend.Start()
		Expect(err).ToNot(HaveOccurred())

		Expect(fakeContainerPool.Pruned).To(BeTrue())
		Expect(fakeContainerPool.KeptContainers).To(Equal(map[string]bool{}))
	})

	Context("when pruning the container pool fails", func() {
		disaster := errors.New("failed to prune")

		BeforeEach(func() {
			fakeContainerPool.PruneError = disaster
		})

		It("returns the error", func() {
			linuxBackend := linux_backend.New(fakeContainerPool, fakeSystemInfo, "")

			err := linuxBackend.Start()
			Expect(err).To(Equal(disaster))
		})
	})
})

var _ = Describe("Stop", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var fakeSystemInfo *fake_system_info.FakeProvider
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		tmpdir, err := ioutil.TempDir(os.TempDir(), "warden-server-test")
		Expect(err).ToNot(HaveOccurred())

		fakeContainerPool = fake_container_pool.New()
		linuxBackend = linux_backend.New(
			fakeContainerPool,
			fakeSystemInfo,
			path.Join(tmpdir, "snapshots"),
		)
	})

	It("takes a snapshot of each container", func() {
		container1, err := linuxBackend.Create(warden.ContainerSpec{Handle: "some-handle"})
		Expect(err).ToNot(HaveOccurred())

		container2, err := linuxBackend.Create(warden.ContainerSpec{Handle: "some-other-handle"})
		Expect(err).ToNot(HaveOccurred())

		linuxBackend.Stop()

		fakeContainer1 := container1.(*fake_backend.FakeContainer)
		fakeContainer2 := container2.(*fake_backend.FakeContainer)
		Expect(fakeContainer1.SavedSnapshots).To(HaveLen(1))
		Expect(fakeContainer2.SavedSnapshots).To(HaveLen(1))
	})

	It("cleans up each container", func() {
		container1, err := linuxBackend.Create(warden.ContainerSpec{Handle: "some-handle"})
		Expect(err).ToNot(HaveOccurred())

		container2, err := linuxBackend.Create(warden.ContainerSpec{Handle: "some-other-handle"})
		Expect(err).ToNot(HaveOccurred())

		linuxBackend.Stop()

		fakeContainer1 := container1.(*fake_backend.FakeContainer)
		fakeContainer2 := container2.(*fake_backend.FakeContainer)
		Expect(fakeContainer1.CleanedUp).To(BeTrue())
		Expect(fakeContainer2.CleanedUp).To(BeTrue())
	})
})

var _ = Describe("Capacity", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var fakeSystemInfo *fake_system_info.FakeProvider
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo = fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(fakeContainerPool, fakeSystemInfo, "")
	})

	It("returns the right capacity values", func() {
		fakeSystemInfo.TotalMemoryResult = 1111
		fakeSystemInfo.TotalDiskResult = 2222
		fakeContainerPool.MaxContainersValue = 42

		capacity, err := linuxBackend.Capacity()
		Ω(err).ShouldNot(HaveOccurred())

		Expect(capacity.MemoryInBytes).To(Equal(uint64(1111)))
		Expect(capacity.DiskInBytes).To(Equal(uint64(2222)))
		Expect(capacity.MaxContainers).To(Equal(uint64(42)))
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
		linuxBackend = linux_backend.New(fakeContainerPool, fakeSystemInfo, "")
	})

	It("creates a container from the pool", func() {
		Expect(fakeContainerPool.CreatedContainers).To(BeEmpty())

		container, err := linuxBackend.Create(warden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())

		Expect(fakeContainerPool.CreatedContainers).To(ContainElement(container))
	})

	It("starts the container", func() {
		container, err := linuxBackend.Create(warden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())
		Expect(container.(*fake_backend.FakeContainer).Started).To(BeTrue())
	})

	It("registers the container", func() {
		container, err := linuxBackend.Create(warden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())

		foundContainer, err := linuxBackend.Lookup(container.Handle())
		Expect(err).ToNot(HaveOccurred())

		Expect(foundContainer).To(Equal(container))
	})

	Context("when creating the container fails", func() {
		disaster := errors.New("failed to create")

		BeforeEach(func() {
			fakeContainerPool.CreateError = disaster
		})

		It("returns the error", func() {
			container, err := linuxBackend.Create(warden.ContainerSpec{})
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(disaster))

			Expect(container).To(BeNil())
		})
	})

	Context("when starting the container fails", func() {
		disaster := errors.New("failed to start")

		BeforeEach(func() {
			fakeContainerPool.ContainerSetup = func(c *fake_backend.FakeContainer) {
				c.StartError = disaster
			}
		})

		It("returns the error", func() {
			container, err := linuxBackend.Create(warden.ContainerSpec{})
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(disaster))

			Expect(container).To(BeNil())
		})

		It("does not register the container", func() {
			_, err := linuxBackend.Create(warden.ContainerSpec{})
			Expect(err).To(HaveOccurred())

			containers, err := linuxBackend.Containers(nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(containers).To(BeEmpty())
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
		linuxBackend = linux_backend.New(fakeContainerPool, fakeSystemInfo, "")

		newContainer, err := linuxBackend.Create(warden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())

		container = newContainer
	})

	It("removes the given container from the pool", func() {
		Expect(fakeContainerPool.DestroyedContainers).To(BeEmpty())

		err := linuxBackend.Destroy(container.Handle())
		Expect(err).ToNot(HaveOccurred())

		Expect(fakeContainerPool.DestroyedContainers).To(ContainElement(container))
	})

	It("unregisters the container", func() {
		err := linuxBackend.Destroy(container.Handle())
		Expect(err).ToNot(HaveOccurred())

		_, err = linuxBackend.Lookup(container.Handle())
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(linux_backend.UnknownHandleError{container.Handle()}))
	})

	Context("when the container does not exist", func() {
		It("returns UnknownHandleError", func() {
			err := linuxBackend.Destroy("bogus-handle")
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(linux_backend.UnknownHandleError{"bogus-handle"}))
		})
	})

	Context("when destroying the container fails", func() {
		disaster := errors.New("failed to destroy")

		BeforeEach(func() {
			fakeContainerPool.DestroyError = disaster
		})

		It("returns the error", func() {
			err := linuxBackend.Destroy(container.Handle())
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(disaster))
		})

		It("does not unregister the container", func() {
			err := linuxBackend.Destroy(container.Handle())
			Expect(err).To(HaveOccurred())

			foundContainer, err := linuxBackend.Lookup(container.Handle())
			Expect(err).ToNot(HaveOccurred())
			Expect(foundContainer).To(Equal(container))
		})
	})
})

var _ = Describe("Lookup", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo := fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(fakeContainerPool, fakeSystemInfo, "")
	})

	It("returns the container", func() {
		container, err := linuxBackend.Create(warden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())

		foundContainer, err := linuxBackend.Lookup(container.Handle())
		Expect(err).ToNot(HaveOccurred())

		Expect(foundContainer).To(Equal(container))
	})

	Context("when the handle is not found", func() {
		It("returns UnknownHandleError", func() {
			foundContainer, err := linuxBackend.Lookup("bogus-handle")
			Expect(err).To(HaveOccurred())
			Expect(foundContainer).To(BeNil())

			Expect(err).To(Equal(linux_backend.UnknownHandleError{"bogus-handle"}))
		})
	})
})

var _ = Describe("Containers", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo := fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(fakeContainerPool, fakeSystemInfo, "")
	})

	It("returns a list of all existing containers", func() {
		container1, err := linuxBackend.Create(warden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())

		container2, err := linuxBackend.Create(warden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())

		containers, err := linuxBackend.Containers(nil)
		Expect(err).ToNot(HaveOccurred())

		Expect(containers).To(ContainElement(container1))
		Expect(containers).To(ContainElement(container2))
	})

	Context("when given properties to filter by", func() {
		It("returns only containers with matching properties", func() {
			container1, err := linuxBackend.Create(warden.ContainerSpec{
				Properties: warden.Properties{"a": "b"},
			})
			Expect(err).ToNot(HaveOccurred())

			container2, err := linuxBackend.Create(warden.ContainerSpec{
				Properties: warden.Properties{"a": "b"},
			})
			Expect(err).ToNot(HaveOccurred())

			container3, err := linuxBackend.Create(warden.ContainerSpec{
				Properties: warden.Properties{"a": "b", "c": "d", "e": "f"},
			})
			Expect(err).ToNot(HaveOccurred())

			containers, err := linuxBackend.Containers(
				warden.Properties{"a": "b", "e": "f"},
			)
			Expect(err).ToNot(HaveOccurred())

			Expect(containers).ToNot(ContainElement(container1))
			Expect(containers).ToNot(ContainElement(container2))
			Expect(containers).To(ContainElement(container3))
		})
	})
})

var _ = Describe("GraceTime", func() {
	var fakeContainerPool *fake_container_pool.FakeContainerPool
	var linuxBackend *linux_backend.LinuxBackend

	BeforeEach(func() {
		fakeContainerPool = fake_container_pool.New()
		fakeSystemInfo := fake_system_info.NewFakeProvider()
		linuxBackend = linux_backend.New(fakeContainerPool, fakeSystemInfo, "")
	})

	It("returns the container's grace time", func() {
		container, err := linuxBackend.Create(warden.ContainerSpec{
			GraceTime: time.Second,
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(linuxBackend.GraceTime(container)).To(Equal(time.Second))
	})
})
