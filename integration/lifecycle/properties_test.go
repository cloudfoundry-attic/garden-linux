package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("A container with properties", func() {
	var container garden.Container

	BeforeEach(func() {
		client = startGarden()

		var err error

		container, err = client.Create(garden.ContainerSpec{
			Properties: garden.Properties{
				"foo": "bar",
				"a":   "b",
			},
		})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())
	})

	Describe("reporting the container's info", func() {
		It("includes the properties", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Properties["foo"]).Should(Equal("bar"))
			Ω(info.Properties["a"]).Should(Equal("b"))

			Ω(info.Properties).Should(HaveLen(2))
		})
	})

	Describe("updating container properties", func() {
		It("can CRUD", func() {
			value, err := container.GetProperty("foo")
			Ω(err).ShouldNot(HaveOccurred())
			Ω(value).Should(Equal("bar"))

			err = container.SetProperty("foo", "baz")
			Ω(err).ShouldNot(HaveOccurred())

			err = container.RemoveProperty("a")
			Ω(err).ShouldNot(HaveOccurred())

			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Properties).Should(Equal(garden.Properties{
				"foo": "baz",
			}))
		})
	})

	Describe("listing container info", func() {
		var undesiredHandles []string

		BeforeEach(func() {
			undesiredContainer, err := client.Create(garden.ContainerSpec{
				Properties: garden.Properties{
					"foo": "baz",
					"a":   "b",
				},
			})

			Ω(err).ShouldNot(HaveOccurred())

			undesiredHandles = append(undesiredHandles, undesiredContainer.Handle())

			undesiredContainer, err = client.Create(garden.ContainerSpec{
				Properties: garden.Properties{
					"baz": "bar",
					"a":   "b",
				},
			})

			Ω(err).ShouldNot(HaveOccurred())

			undesiredHandles = append(undesiredHandles, undesiredContainer.Handle())
		})

		AfterEach(func() {
			for _, handle := range undesiredHandles {
				err := client.Destroy(handle)
				Ω(err).ShouldNot(HaveOccurred())
			}
		})

		It("can filter by property", func() {
			containers, err := client.Containers(garden.Properties{"foo": "bar"})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(containers).Should(HaveLen(1))
			Ω(containers[0].Handle()).Should(Equal(container.Handle()))

			containers, err = client.Containers(garden.Properties{"matthew": "mcconaughey"})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(containers).Should(BeEmpty())
		})
	})
})
