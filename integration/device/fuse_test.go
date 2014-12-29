package device_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden/api"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Fuse", func() {
	var container api.Container
	var privilegedContainer bool
	var privilegedProcess bool

	BeforeEach(func() {
		privilegedContainer = true
		privilegedProcess = true
	})

	JustBeforeEach(func() {
		client = startGarden()

		var err error
		container, err = client.Create(api.ContainerSpec{
			RootFSPath: fuseRootFSPath,
			Privileged: privilegedContainer,
		})
		Ω(err).ShouldNot(HaveOccurred())
	})

	Describe("/dev/fuse", func() {
		It("is a character special device file", func() {
			process, err := container.Run(api.ProcessSpec{
				Privileged: privilegedProcess,
				Path:       "/usr/bin/test",
				Args:       []string{"-c", "/dev/fuse"},
			}, api.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process.Wait()).Should(Equal(0), "/dev/fuse cannot be found or is not a character special device.")
		})

		Context("in a privileged Container", func() {
			BeforeEach(func() {
				privilegedContainer = true
			})

			Context("a privileged process", func() {
				BeforeEach(func() {
					privilegedProcess = true
				})

				It("can mount a fuse filesystem", func() {
					canCreateAndUseFuseFileSystem(container, privilegedProcess)
				})
			})

			Context("a non-privileged process", func() {
				BeforeEach(func() {
					privilegedProcess = false
				})

				It("can mount a fuse filesystem", func() {
					canCreateAndUseFuseFileSystem(container, privilegedProcess)
				})
			})
		})
	})
})

func canCreateAndUseFuseFileSystem(container api.Container, privilegedProcess bool) {
	mountpoint := "/tmp/fuse-test"

	process, err := container.Run(api.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "mkdir",
		Args:       []string{"-p", mountpoint},
	}, api.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0), "Could not make temporary directory!")

	process, err = container.Run(api.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "ls",
		Args:       []string{"-lA", "/dev/fuse", "/bin/fusermount", "/usr/bin/hellofs"},
	}, api.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0), "Failed to ls various files.")

	process, err = container.Run(api.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "/usr/bin/hellofs",
		Args:       []string{mountpoint},
	}, api.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0), "Failed to mount hello filesystem.")

	stdout := gbytes.NewBuffer()
	process, err = container.Run(api.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "cat",
		Args:       []string{filepath.Join(mountpoint, "hello")},
	}, api.ProcessIO{Stdout: stdout, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0), "Failed to find hello file.")
	Ω(stdout).Should(gbytes.Say("Hello World!"))

	process, err = container.Run(api.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "fusermount",
		Args:       []string{"-u", mountpoint},
	}, api.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0), "Failed to unmount user filesystem.")

	stdout2 := gbytes.NewBuffer()
	process, err = container.Run(api.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "ls",
		Args:       []string{mountpoint},
	}, api.ProcessIO{Stdout: stdout2, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0))
	Ω(stdout2).ShouldNot(gbytes.Say("hello"), "Fuse filesystem appears still to be visible after being unmounted.")
}
