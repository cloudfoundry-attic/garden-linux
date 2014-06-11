package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("A container with properties", func() {
	var container warden.Container

	BeforeEach(func() {
		client = startWarden()

		var err error

		container, err = client.Create(warden.ContainerSpec{
			Properties: warden.Properties{
				"foo": "bar",
				"a":   "b",
			},
		})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("when reporting the container's info", func() {
		It("includes the properties", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Properties["foo"]).Should(Equal("bar"))
			Ω(info.Properties["a"]).Should(Equal("b"))

			Ω(info.Properties).Should(HaveLen(2))
		})
	})

	Describe("when listing container info", func() {
		var undesiredHandles []string

		BeforeEach(func() {
			undesiredContainer, err := client.Create(warden.ContainerSpec{
				Properties: warden.Properties{
					"foo": "baz",
					"a":   "b",
				},
			})

			Expect(err).ToNot(HaveOccurred())

			undesiredHandles = append(undesiredHandles, undesiredContainer.Handle())

			undesiredContainer, err = client.Create(warden.ContainerSpec{
				Properties: warden.Properties{
					"baz": "bar",
					"a":   "b",
				},
			})

			Expect(err).ToNot(HaveOccurred())

			undesiredHandles = append(undesiredHandles, undesiredContainer.Handle())
		})

		AfterEach(func() {
			for _, handle := range undesiredHandles {
				err := client.Destroy(handle)
				Ω(err).ShouldNot(HaveOccurred())
			}
		})

		It("can filter by property", func() {
			containers, err := client.Containers(warden.Properties{"foo": "bar"})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(containers).Should(HaveLen(1))
			Ω(containers[0].Handle()).Should(Equal(container.Handle()))

			containers, err = client.Containers(warden.Properties{"matthew": "mcconaughey"})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(containers).Should(BeEmpty())
		})
	})
})
