package lifecycle_test

import (
	"io"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Resource limits", func() {
	var (
		container           garden.Container
		privilegedContainer bool
	)

	JustBeforeEach(func() {
		var err error

		client = startGarden()

		container, err = client.Create(garden.ContainerSpec{
			Privileged: privilegedContainer,
		})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Specific resource limits", func() {
		Context("CPU rlimit", func() {
			Context("with a privileged container", func() {
				BeforeEach(func() {
					privilegedContainer = true
				})

				It("rlimits can be set", func() {
					var cpu uint64 = 9000
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						User: "root",
						Args: []string{"-c", "ulimit -t"},
						Limits: garden.ResourceLimits{
							Cpu: &cpu,
						},
					}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("9000"))
					Expect(process.Wait()).To(Equal(0))
				})
			})

			Context("with a non-privileged container", func() {
				BeforeEach(func() {
					privilegedContainer = false
				})

				It("rlimits can be set", func() {
					var cpu uint64 = 9000
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						User: "root",
						Args: []string{"-c", "ulimit -t"},
						Limits: garden.ResourceLimits{
							Cpu: &cpu,
						},
					}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("9000"))
					Expect(process.Wait()).To(Equal(0))
				})
			})
		})

		Context("FSIZE rlimit", func() {
			Context("with a privileged container", func() {
				BeforeEach(func() {
					privilegedContainer = true
				})

				It("rlimits can be set", func() {
					var fsize uint64 = 4194304
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						User: "root",
						Args: []string{"-c", "ulimit -f"},
						Limits: garden.ResourceLimits{
							Fsize: &fsize,
						},
					}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("8192"))
					Expect(process.Wait()).To(Equal(0))
				})
			})

			Context("with a non-privileged container", func() {
				BeforeEach(func() {
					privilegedContainer = false
				})

				It("rlimits can be set", func() {
					var fsize uint64 = 4194304
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						User: "root",
						Args: []string{"-c", "ulimit -f"},
						Limits: garden.ResourceLimits{
							Fsize: &fsize,
						},
					}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("8192"))
					Expect(process.Wait()).To(Equal(0))
				})
			})
		})

		Context("NOFILE rlimit", func() {
			Context("with a privileged container", func() {
				BeforeEach(func() {
					privilegedContainer = true
				})

				It("rlimits can be set", func() {
					var nofile uint64 = 524288
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						User: "root",
						Args: []string{"-c", "ulimit -n"},
						Limits: garden.ResourceLimits{
							Nofile: &nofile,
						},
					}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("524288"))
					Expect(process.Wait()).To(Equal(0))
				})
			})

			Context("with a non-privileged container", func() {
				BeforeEach(func() {
					privilegedContainer = false
				})

				It("rlimits can be set", func() {
					var nofile uint64 = 524288
					stdout := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						User: "root",
						Args: []string{"-c", "ulimit -n"},
						Limits: garden.ResourceLimits{
							Nofile: &nofile,
						},
					}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("524288"))
					Expect(process.Wait()).To(Equal(0))
				})
			})
		})
	})
})
