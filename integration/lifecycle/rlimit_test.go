package lifecycle_test

import (
	"io"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Resource limits", func() {
	var (
		container           garden.Container
		privilegedContainer bool
		rlimitValue         uint64
		prevRlimit          syscall.Rlimit
		rlimitResource      int
	)

	JustBeforeEach(func() {
		err := syscall.Getrlimit(rlimitResource, &prevRlimit)
		Ω(err).ShouldNot(HaveOccurred())

		rlimit := syscall.Rlimit{Cur: rlimitValue, Max: rlimitValue}
		err = syscall.Setrlimit(rlimitResource, &rlimit)
		Ω(err).ShouldNot(HaveOccurred())

		client = startGarden()
		container, err = client.Create(garden.ContainerSpec{
			Privileged: privilegedContainer,
		})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		err = syscall.Setrlimit(rlimitResource, &prevRlimit)
		Ω(err).ShouldNot(HaveOccurred())
	})

	Context("NOFILE rlimit", func() {
		BeforeEach(func() {
			rlimitResource = syscall.RLIMIT_NOFILE
			rlimitValue = 100
		})

		Context("with a privileged container", func() {
			BeforeEach(func() {
				privilegedContainer = true
			})

			It("rlimits can be set", func() {
				var nofile uint64 = 1000
				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					Path: "sh",
					Args: []string{"-c", "ulimit -n"},
					Limits: garden.ResourceLimits{
						Nofile: &nofile,
					},
				}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("1000"))
				Ω(process.Wait()).Should(Equal(0))
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
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("1000"))
				Ω(process.Wait()).Should(Equal(0))
			})
		})
	})

	Context("AS rlimit", func() {
		BeforeEach(func() {
			rlimitResource = syscall.RLIMIT_AS
			rlimitValue = 2147483648
		})

		Context("with a privileged container", func() {
			BeforeEach(func() {
				privilegedContainer = true
			})

			It("rlimits can be set", func() {
				var as uint64 = 4294967296
				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					Path: "sh",
					Args: []string{"-c", "ulimit -v"},
					Limits: garden.ResourceLimits{
						As: &as,
					},
				}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("4194304"))
				Ω(process.Wait()).Should(Equal(0))
			})
		})

		Context("with a non-privileged container", func() {
			BeforeEach(func() {
				privilegedContainer = false
			})

			It("rlimits can be set", func() {
				var as uint64 = 4294967296
				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					Path: "sh",
					User: "vcap",
					Args: []string{"-c", "ulimit -v"},
					Limits: garden.ResourceLimits{
						As: &as,
					},
				}, garden.ProcessIO{Stdout: io.MultiWriter(stdout, GinkgoWriter), Stderr: GinkgoWriter})
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("4194304"))
				Ω(process.Wait()).Should(Equal(0))
			})
		})
	})
})
