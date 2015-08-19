package device_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Fuse", func() {
	var container garden.Container
	var privilegedContainer bool
	var user string

	BeforeEach(func() {
		privilegedContainer = true
		user = "root"
	})

	JustBeforeEach(func() {
		client = startGarden()

		var err error
		container, err = client.Create(garden.ContainerSpec{
			RootFSPath: fuseRootFSPath,
			Privileged: privilegedContainer,
		})
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("/dev/fuse", func() {
		It("is a character special device file", func() {
			process, err := container.Run(garden.ProcessSpec{
				User: user,
				Path: "/usr/bin/test",
				Args: []string{"-c", "/dev/fuse"},
			}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
			Expect(err).ToNot(HaveOccurred())

			Expect(process.Wait()).To(Equal(0), "/dev/fuse cannot be found or is not a character special device.")
		})

		Context("in a privileged Container", func() {
			BeforeEach(func() {
				privilegedContainer = true
			})

			Context("a privileged process", func() {
				BeforeEach(func() {
					user = "root"
				})

				It("can mount a fuse filesystem", func() {
					canCreateAndUseFuseFileSystem(container, user)
				})
			})

			Context("a non-privileged process", func() {
				BeforeEach(func() {
					user = "alice"
				})

				It("can mount a fuse filesystem", func() {
					canCreateAndUseFuseFileSystem(container, user)
				})
			})
		})
	})
})

func canCreateAndUseFuseFileSystem(container garden.Container, user string) {
	mountpoint := "/tmp/fuse-test"

	process, err := container.Run(garden.ProcessSpec{
		User: user,
		Path: "mkdir",
		Args: []string{"-p", mountpoint},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Expect(err).ToNot(HaveOccurred())
	Expect(process.Wait()).To(Equal(0), "Could not make temporary directory!")

	process, err = container.Run(garden.ProcessSpec{
		User: user,
		Path: "/usr/bin/hellofs",
		Args: []string{mountpoint},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Expect(err).ToNot(HaveOccurred())
	Expect(process.Wait()).To(Equal(0), "Failed to mount hello filesystem.")

	stdout := gbytes.NewBuffer()
	process, err = container.Run(garden.ProcessSpec{
		User: user,
		Path: "cat",
		Args: []string{filepath.Join(mountpoint, "hello")},
	}, garden.ProcessIO{Stdout: stdout, Stderr: GinkgoWriter})
	Expect(err).ToNot(HaveOccurred())
	Expect(process.Wait()).To(Equal(0), "Failed to find hello file.")
	Expect(stdout).To(gbytes.Say("Hello World!"))

	process, err = container.Run(garden.ProcessSpec{
		User: user,
		Path: "fusermount",
		Args: []string{"-u", mountpoint},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Expect(err).ToNot(HaveOccurred())
	Expect(process.Wait()).To(Equal(0), "Failed to unmount user filesystem.")

	stdout2 := gbytes.NewBuffer()
	process, err = container.Run(garden.ProcessSpec{
		User: user,
		Path: "ls",
		Args: []string{mountpoint},
	}, garden.ProcessIO{Stdout: stdout2, Stderr: GinkgoWriter})
	Expect(err).ToNot(HaveOccurred())
	Expect(process.Wait()).To(Equal(0))
	Expect(stdout2).ToNot(gbytes.Say("hello"), "Fuse filesystem appears still to be visible after being unmounted.")
}
