package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = PDescribe("Limits", func() {
	const BTRFS_WAIT_TIME = 90

	var container garden.Container
	var startGardenArgs []string

	var privilegedContainer bool
	var rootfs string

	JustBeforeEach(func() {
		var err error
		client = startGarden(startGardenArgs...)
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
		startGardenArgs = []string{}
	})

	Describe("LimitDisk", func() {
		Context("with quotas disabled", func() {
			BeforeEach(func() {
				startGardenArgs = []string{"-disableQuotas=true"}
				rootfs = runner.RootFSPath
				privilegedContainer = true
			})

			Context("and there is a disk limit", func() {
				quotaLimit := garden.DiskLimits{
					ByteSoft: 5 * 1024 * 1024,
					ByteHard: 5 * 1024 * 1024,
				}

				JustBeforeEach(func() {
					Expect(container.LimitDisk(quotaLimit)).To(Succeed())
				})

				It("reports the disk limit size of the container as zero", func() {
					limit, err := container.CurrentDiskLimits()
					Expect(err).ToNot(HaveOccurred())
					Expect(limit).To(Equal(garden.DiskLimits{}))
				})

				Context("and it runs a process that exceeds the limit", func() {
					It("does not kill the process", func() {
						dd, err := container.Run(garden.ProcessSpec{
							User: "alice",
							Path: "dd",
							Args: []string{"if=/dev/zero", "of=/tmp/some-file", "bs=1M", "count=6"},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())
						Expect(dd.Wait()).To(Equal(0))
					})
				})
			})
		})
	})
})
