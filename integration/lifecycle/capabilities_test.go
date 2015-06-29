package lifecycle_test

import (
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = FDescribe("Capabilities tool", func() {
	var (
		container  garden.Container
		privileged bool
	)

	JustBeforeEach(func() {
		client = startGarden()

		bindMount := garden.BindMount{
			SrcPath: filepath.Dir(capabilityTestBin),
			DstPath: "/tools",
			Mode:    garden.BindMountModeRO,
			Origin:  garden.BindMountOriginHost,
		}

		var err error
		container, err = client.Create(garden.ContainerSpec{
			BindMounts: []garden.BindMount{bindMount},
			Privileged: privileged,
		})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if container != nil {
			Expect(client.Destroy(container.Handle())).To(Succeed())
		}
	})

	Context("in an unprivileged container", func() {

		BeforeEach(func() {
			privileged = false
		})

		Context("when a process is run as root", func() {
			It("should detect that CAP_CHOWN was dropped", func() {
				stderr := gbytes.NewBuffer()
				process, err := container.Run(checkCap("CAP_CHOWN", "root"), garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: stderr,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(1))
				Eventually(string(stderr.Contents())).Should(ContainSubstring("operation not permitted"))
			})
		})

		Context("when a process is run as a non-root user", func() {
			It("should detect that CAP_CHOWN was dropped", func() {
				stderr := gbytes.NewBuffer()
				process, err := container.Run(checkCap("CAP_CHOWN", "vcap"), garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: stderr,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(1))
				Eventually(string(stderr.Contents())).Should(ContainSubstring("operation not permitted"))
			})
		})
	})

	Context("in a privileged container", func() {

		BeforeEach(func() {
			privileged = true
		})

		Context("when a process is run as root", func() {
			It("should detect that CAP_CHOWN was not dropped", func() {
				stderr := gbytes.NewBuffer()
				process, err := container.Run(checkCap("CAP_CHOWN", "root"), garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: stderr,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(0))
			})
		})

		Context("when a process is run as a non-root user", func() {
			It("should detect that CAP_CHOWN was dropped", func() {
				stderr := gbytes.NewBuffer()
				process, err := container.Run(checkCap("CAP_CHOWN", "vcap"), garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: stderr,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(1))
				Eventually(string(stderr.Contents())).Should(ContainSubstring("operation not permitted"))
			})
		})
	})
})

func checkCap(cap, user string) garden.ProcessSpec {
	return garden.ProcessSpec{
		User: user,
		Path: "/tools/capability",
		Args: []string{cap},
	}
}
