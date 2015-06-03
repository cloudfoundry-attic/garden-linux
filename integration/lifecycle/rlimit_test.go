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

	Context("when setting all rlimits to minimum values", func() {
		It("succeeds", func(done Done) {
			// Experimental minimum values tend to produce flakes.
			fudgeFactor := 1.50

			var (
				val0 uint64 = 0
				// Number of open files
				valNofile uint64 = uint64(4 * fudgeFactor)
				// Memory limits
				valAs    uint64 = uint64(4194304 * fudgeFactor)
				valData  uint64 = uint64(8192 * fudgeFactor)
				valStack uint64 = uint64(11264 * fudgeFactor)
			)

			rlimits := garden.ResourceLimits{
				// Memory limits
				As:    &valAs,
				Data:  &valData,
				Stack: &valStack,
				// Number of open files
				Nofile: &valNofile,
				// Can be zero
				Core:       &val0,
				Cpu:        &val0,
				Fsize:      &val0,
				Locks:      &val0,
				Memlock:    &val0,
				Msgqueue:   &val0,
				Nice:       &val0,
				Nproc:      &val0,
				Rss:        &val0,
				Rtprio:     &val0,
				Sigpending: &val0,
			}

			proc, err := container.Run(
				garden.ProcessSpec{
					Path:   "echo",
					Args:   []string{"Hello world"},
					User:   "root",
					Limits: rlimits,
				},
				garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				},
			)
			Expect(err).ToNot(HaveOccurred())

			Expect(proc.Wait()).To(Equal(0))

			close(done)
		}, 10)
	})

	Describe("Specific resource limits", func() {
		Context("AS rlimit", func() {
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
