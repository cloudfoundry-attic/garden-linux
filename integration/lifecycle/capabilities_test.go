package lifecycle_test

import (
	"os"

	"github.com/cloudfoundry-incubator/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Capabilities", func() {
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

	Context("by default (unprivileged)", func() {
		It("drops capabilities, including CAP_SYS_ADMIN, and therefore cannot mount", func() {
			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "mount",
				Args: []string{"-t", "tmpfs", "/tmp"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(process.Wait()).ToNot(Equal(0))
		})

		Context("when the process is run as non-root user", func() {
			BeforeEach(func() {
				rootfs = os.Getenv("GARDEN_NESTABLE_TEST_ROOTFS")
			})

			It("does not have certain capabilities, even when user changes to root", func() {
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "sh",
					Args: []string{"-c", `echo "ALL            ALL = (ALL) NOPASSWD: ALL" >> /etc/sudoers`},
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(0))

				process, err = container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "sudo",
					Args: []string{"chown", "-R", "vcap", "/tmp"},
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).ToNot(Equal(0))
			})
		})
	})
})
