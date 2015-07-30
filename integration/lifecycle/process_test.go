package lifecycle_test

import (
	"io"

	"github.com/cloudfoundry-incubator/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = FDescribe("Process", func() {
	var container garden.Container

	BeforeEach(func() {
		var err error
		client = startGarden()
		container, err = client.Create(garden.ContainerSpec{
			RootFSPath: "docker:///cloudfoundry/preexisting_users",
		})

		Expect(err).ToNot(HaveOccurred())
	})

	Describe("working directory", func() {
		Context("when user has access to working directory", func() {
			Context("when working directory exists", func() {
				It("a process is spawned", func() {
					out := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						User: "alice",
						Dir:  "/home/alice",
						Path: "pwd",
					}, garden.ProcessIO{
						Stdout: out,
						Stderr: out,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(process.Wait()).To(Equal(0))
					Eventually(out).Should(gbytes.Say("/home/alice"))
				})
			})
		})

		Context("when user has access to create working directory", func() {
			Context("when working directory does not exist", func() {
				It("a process is spawned", func() {
					out := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						User: "alice",
						Dir:  "/home/alice/nonexistent",
						Path: "pwd",
					}, garden.ProcessIO{
						Stdout: out,
						Stderr: GinkgoWriter,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(process.Wait()).To(Equal(0))
					Eventually(out).Should(gbytes.Say("/home/alice/nonexistent"))
				})
			})
		})

		Context("when user does not have access to working directory", func() {
			Context("when working directory does exist", func() {
				It("returns an error", func() {
					out := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						User: "alice",
						Dir:  "/root",
						Path: "ls",
					}, garden.ProcessIO{
						Stdout: GinkgoWriter,
						Stderr: io.MultiWriter(GinkgoWriter, out),
					})

					Expect(err).ToNot(HaveOccurred())

					exitStatus, err := process.Wait()
					Expect(exitStatus).ToNot(Equal(0))
					Expect(out).To(gbytes.Say("proc_starter: ExecAsUser: system: invalid working directory: /root"))
				})
			})

			Context("when working directory does not exist", func() {
				It("returns an error", func() {
					out := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						User: "alice",
						Dir:  "/home/bob/nonexistent",
						Path: "pwd",
					}, garden.ProcessIO{
						Stdout: GinkgoWriter,
						Stderr: io.MultiWriter(GinkgoWriter, out),
					})

					Expect(err).ToNot(HaveOccurred())
					exitStatus, err := process.Wait()
					Expect(exitStatus).ToNot(Equal(0))
					Expect(out).To(gbytes.Say("proc_starter: ExecAsUser: system: mkdir /home/bob/nonexistent: permission denied"))
				})
			})
		})
	})
})
