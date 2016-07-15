package graph_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"code.cloudfoundry.org/garden"
)

var _ = Describe("VFS", func() {
	var container garden.Container

	BeforeEach(func() {
		client = startGarden("-graphDriver", "vfs")
	})

	It("logs a warning on start", func() {
		Eventually(client.Buffer).Should(gbytes.Say("unsupported-graph-driver.*vfs"))
	})

	Describe("a privileged container", func() {
		JustBeforeEach(func() {
			var err error
			container, err = client.Create(garden.ContainerSpec{
				RootFSPath: rootFSPath,
				Privileged: true,
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("can run a process", func() {
			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "true",
			}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
			Expect(err).ToNot(HaveOccurred())

			Expect(process.Wait()).To(Equal(0))
		})
	})

	Describe("an unprivileged container", func() {
		JustBeforeEach(func() {
			var err error
			container, err = client.Create(garden.ContainerSpec{
				RootFSPath: rootFSPath,
				Privileged: false,
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("can run a process", func() {
			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "true",
			}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
			Expect(err).ToNot(HaveOccurred())

			Expect(process.Wait()).To(Equal(0))
		})
	})
})
