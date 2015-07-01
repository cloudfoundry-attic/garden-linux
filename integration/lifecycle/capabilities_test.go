package lifecycle_test

import (
	"os"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = FDescribe("Capabilities", func() {
	var (
		container  garden.Container
		bindMounts []garden.BindMount
		privileged bool
		rootfs     string
	)

	JustBeforeEach(func() {
		client = startGarden()

		var err error
		container, err = client.Create(garden.ContainerSpec{
			BindMounts: bindMounts,
			Privileged: privileged,
			RootFSPath: rootfs})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if container != nil {
			Expect(client.Destroy(container.Handle())).To(Succeed())
		}
	})

	BeforeEach(func() {
		privileged = false
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

		Context("when capability tool is executed", func() {
			BeforeEach(func() {
				bindMount := garden.BindMount{
					SrcPath: filepath.Dir(capabilityTestBin),
					DstPath: "/tools",
					Mode:    garden.BindMountModeRO,
					Origin:  garden.BindMountOriginHost,
				}

				bindMounts = append(bindMounts, bindMount)
			})

			It("should not be able to chown a file, because CAP_CHOWN is dropped", func() {
				stderr := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "/tools/capability",
					Args: []string{"inspect", "CAP_CHOWN"},
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: stderr,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(1))
				Eventually(string(stderr.Contents())).Should(ContainSubstring("operation not permitted"))
			})

			It("should not be able to set group id, because CAP_SETUID is dropped", func() {
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "/tools/capability",
					Args: []string{"inspect", "CAP_SETUID"},
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(201))
			})

			It("should not be able to set group id, because CAP_SETGID is dropped", func() {
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "/tools/capability",
					Args: []string{"inspect", "CAP_SETGID"},
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(202))
			})

			It("should not be able to set system clock, because CAP_SYS_TIME is dropped", func() {
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "/tools/capability",
					Args: []string{"inspect", "CAP_SYS_TIME"},
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(203))
			})
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
