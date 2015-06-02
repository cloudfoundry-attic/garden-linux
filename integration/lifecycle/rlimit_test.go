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
		client = startGarden()
		var err error
		container, err = client.Create(garden.ContainerSpec{
			Privileged: privilegedContainer,
		})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("NOFILE rlimit", func() {

		Context("with a privileged container", func() {
			BeforeEach(func() {
				privilegedContainer = true
			})

			It("rlimits can be set", func() {
				var nofile uint64 = 1000
				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "sh",
					Args: []string{"-c", "ulimit -n"},
					Limits: garden.ResourceLimits{
						Nofile: &nofile,
					},
				}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("1000"))
				Expect(process.Wait()).To(Equal(0))
			})
		})

		Context("with a non-privileged container", func() {
			BeforeEach(func() {
				privilegedContainer = false
			})

			It("rlimits can be set", func() {
				var nofile uint64 = 1000
				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					Path: "sh",
					User: "vcap",
					Args: []string{"-c", "ulimit -n"},
					Limits: garden.ResourceLimits{
						Nofile: &nofile,
					},
				}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("1000"))
				Expect(process.Wait()).To(Equal(0))
			})
		})
	})

	Describe("AS rlimit", func() {

		Context("with a privileged container", func() {
			BeforeEach(func() {
				privilegedContainer = true
			})

			It("rlimits can be set", func() {
				as := uint64(4294967296)
				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "sh",
					Args: []string{"-c", "ulimit -v"},
					Limits: garden.ResourceLimits{
						As: &as,
					},
				}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("4194304"))
				Expect(process.Wait()).To(Equal(0))
			})
		})

		Context("with a non-privileged container", func() {
			BeforeEach(func() {
				privilegedContainer = false
			})

			It("rlimits can be set", func() {
				as := uint64(4294967296)
				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "sh",
					Args: []string{"-c", "ulimit -v"},
					Limits: garden.ResourceLimits{
						As: &as,
					},
				}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("4194304"))
				Expect(process.Wait()).To(Equal(0))
			})
		})
	})
})
