package lifecycle_test

import (
	"io"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/gbytes"

	"github.com/cloudfoundry-incubator/garden"
)

var _ = Describe("LANG environment variable", func() {
	var container garden.Container

	JustBeforeEach(func() {
		client = startGarden()

		var err error

		container, err = client.Create(garden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		if container != nil {
			err := client.Destroy(container.Handle())
			Ω(err).ShouldNot(HaveOccurred())
		}
	})

	var lang string
	var langBefore string

	JustBeforeEach(func() {
		langBefore = os.Getenv("LANG")

		if lang == "" {
			os.Unsetenv("LANG")
		} else {
			os.Setenv("LANG", lang)
		}

		if lang != langBefore {
			restartGarden()
		}
	})

	AfterEach(func() {
		if langBefore == "" {
			os.Unsetenv("LANG")
		} else {
			os.Setenv("LANG", langBefore)
		}
	})

	Context("when the host does not have a LANG set", func() {
		BeforeEach(func() {
			lang = ""
		})

		It("uses the default of en_US.UTF-8", func() {
			stdout := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo $LANG"},
			}, garden.ProcessIO{
				Stdout: io.MultiWriter(stdout, GinkgoWriter),
				Stderr: GinkgoWriter,
			})
			Ω(err).ShouldNot(HaveOccurred())

			status, err := process.Wait()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(status).Should(Equal(0))

			Ω(stdout).Should(gbytes.Say("en_US.UTF-8"))
		})
	})

	Context("when the host has a LANG set", func() {
		BeforeEach(func() {
			lang = "C"
		})

		It("is used in the container", func() {
			stdout := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo $LANG"},
			}, garden.ProcessIO{
				Stdout: io.MultiWriter(stdout, GinkgoWriter),
				Stderr: GinkgoWriter,
			})
			Ω(err).ShouldNot(HaveOccurred())

			status, err := process.Wait()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(status).Should(Equal(0))

			Ω(stdout).Should(gbytes.Say("C"))
		})

		It("can be overridden by the user", func() {
			stdout := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo $LANG"},
				Env: []string{
					"LANG=en_US.ASCII",
				},
			}, garden.ProcessIO{
				Stdout: io.MultiWriter(stdout, GinkgoWriter),
				Stderr: GinkgoWriter,
			})
			Ω(err).ShouldNot(HaveOccurred())

			status, err := process.Wait()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(status).Should(Equal(0))

			Ω(stdout).Should(gbytes.Say("en_US.ASCII"))
		})
	})
})
