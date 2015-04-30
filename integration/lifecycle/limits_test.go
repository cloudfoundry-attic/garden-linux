package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Limits", func() {
	var container garden.Container

	var privilegedContainer bool
	var rootfs string

	JustBeforeEach(func() {
		client = startGarden()

		var err error

		container, err = client.Create(garden.ContainerSpec{Privileged: privilegedContainer, RootFSPath: rootfs})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if container != nil {
			Expect(client.Destroy(container.Handle())).To(Succeed())
		}
	})

	BeforeEach(func() {
		privilegedContainer = false
		rootfs = ""
	})

	Context("with a memory limit", func() {
		JustBeforeEach(func() {
			err := container.LimitMemory(garden.MemoryLimits{
				LimitInBytes: 64 * 1024 * 1024,
			})
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when the process writes too much to /dev/shm", func() {
			It("is killed", func() {
				process, err := container.Run(garden.ProcessSpec{
					Path: "dd",
					Args: []string{"if=/dev/urandom", "of=/dev/shm/too-big", "bs=1M", "count=65"},
				}, garden.ProcessIO{})
				Expect(err).ToNot(HaveOccurred())

				Expect(process.Wait()).ToNot(Equal(0))
			})
		})
	})

	FDescribe("Disk quotas", func() {
		Context("on a privileged Docker container", func() {
			BeforeEach(func() {
				privilegedContainer = true
				rootfs = "docker:///busybox"
			})

			Context("when there is a disk quota", func() {
				JustBeforeEach(func() {
					Expect(container.LimitDisk(garden.DiskLimits{
						ByteHard: 5 * 1024 * 1024,
					})).To(Succeed())
				})

				Context("and run a process that exceeds the quota as root", func() {
					It("kills the process", func() {
						dd, err := container.Run(garden.ProcessSpec{
							User: "root",
							Path: "dd",
							Args: []string{"if=/dev/zero", "of=/home/bob/test", "count=102400"},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())
						Expect(dd.Wait()).ToNot(Equal(0))
					})
				})

				Context("and run a process that exceeds the quota as a new user", func() {
					It("kills the process", func() {
						addUser, err := container.Run(garden.ProcessSpec{
							User: "root",
							Path: "adduser",
							Args: []string{"-D", "bob"},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())
						Expect(addUser.Wait()).To(Equal(0))

						dd, err := container.Run(garden.ProcessSpec{
							User: "bob",
							Path: "dd",
							Args: []string{"if=/dev/zero", "of=/home/bob/test", "count=102400"},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())
						Expect(dd.Wait()).ToNot(Equal(0))
					})
				})
			})
		})
	})
})
