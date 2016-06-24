package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Logging", func() {
	var container garden.Container
	var containerSpec garden.ContainerSpec

	BeforeEach(func() {
		containerSpec = garden.ContainerSpec{}
	})

	JustBeforeEach(func() {
		var err error
		client = startGarden()
		container, err = client.Create(containerSpec)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when container is created", func() {
		BeforeEach(func() {
			containerSpec = garden.ContainerSpec{
				Handle: "kumquat",
				Env:    []string{"PASSWORD=MY_SECRET"},
				Properties: garden.Properties{
					"super": "banana",
				},
			}
		})

		It("should log before and after starting with the container handle", func() {
			Eventually(client).Should(gbytes.Say(`container.start.starting","log_level":0,"data":{"handle":"kumquat"`))
			Eventually(client).Should(gbytes.Say(`container.start.ended","log_level":0,"data":{"handle":"kumquat"`))
		})

		It("should not log any environment variables", func() {
			Consistently(client).ShouldNot(gbytes.Say("PASSWORD"))
			Consistently(client).ShouldNot(gbytes.Say("MY_SECRET"))
		})

		It("should not log any properties", func() {
			Consistently(client).ShouldNot(gbytes.Say("super"))
			Consistently(client).ShouldNot(gbytes.Say("banana"))
		})

		Context("from a docker url", func() {
			BeforeEach(func() {
				containerSpec.RootFSPath = "docker:///cfgarden/with-volume"
			})

			It("should not log any environment variables", func() {
				Consistently(client).ShouldNot(gbytes.Say("test-from-dockerfile"))
			})
		})
	})

	Context("when container spawn a new process", func() {
		It("should not log any environment variables and command line arguments", func() {
			process, err := container.Run(garden.ProcessSpec{
				User: "alice",
				Path: "echo",
				Args: []string{"-username", "banana"},
				Env:  []string{"PASSWORD=MY_SECRET"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			Consistently(client).ShouldNot(gbytes.Say("PASSWORD"))
			Consistently(client).ShouldNot(gbytes.Say("MY_SECRET"))
			Consistently(client).ShouldNot(gbytes.Say("-username"))
			Consistently(client).ShouldNot(gbytes.Say("banana"))
		})
	})

	Context("when working with properties", func() {
		BeforeEach(func() {
			containerSpec = garden.ContainerSpec{
				Properties: garden.Properties{
					"super": "banana",
				},
			}
		})

		It("should not log the properties when we are getting them", func() {
			_, err := container.Properties()
			Expect(err).ToNot(HaveOccurred())

			Consistently(client).ShouldNot(gbytes.Say("super"))
			Consistently(client).ShouldNot(gbytes.Say("banana"))
		})

		It("should not log the properties when we are setting them", func() {
			err := container.SetProperty("super", "banana")
			Expect(err).ToNot(HaveOccurred())

			Consistently(client).ShouldNot(gbytes.Say("super"))
			Consistently(client).ShouldNot(gbytes.Say("banana"))
		})
	})
})
