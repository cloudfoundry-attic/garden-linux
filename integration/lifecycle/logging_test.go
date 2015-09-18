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
				Env: []string{"PASSWORD=MY_SECRET"},
				Properties: garden.Properties{
					"super": "banana",
				},
			}
		})

		It("should not log any environment variables", func() {
			Expect(client).ToNot(gbytes.Say("PASSWORD"))
			Expect(client).ToNot(gbytes.Say("MY_SECRET"))
		})

		It("should not log any properties", func() {
			Expect(client).ToNot(gbytes.Say("super"))
			Expect(client).ToNot(gbytes.Say("banana"))
		})

		Context("from a docker url", func() {
			BeforeEach(func() {
				containerSpec.RootFSPath = "docker:///cloudfoundry/with-volume"
			})

			It("should not log any environment variables", func() {
				Expect(client).ToNot(gbytes.Say("test-from-dockerfile"))
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

			Expect(client).ToNot(gbytes.Say("PASSWORD"))
			Expect(client).ToNot(gbytes.Say("MY_SECRET"))
			Expect(client).ToNot(gbytes.Say("-username"))
			Expect(client).ToNot(gbytes.Say("banana"))
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

			Expect(client).ToNot(gbytes.Say("super"))
			Expect(client).ToNot(gbytes.Say("banana"))
		})

		It("should not log the properties when we are setting them", func() {
			err := container.SetProperty("super", "banana")
			Expect(err).ToNot(HaveOccurred())

			Expect(client).ToNot(gbytes.Say("super"))
			Expect(client).ToNot(gbytes.Say("banana"))
		})
	})
})
