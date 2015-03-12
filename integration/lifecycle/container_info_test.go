package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Container information", func() {

	BeforeEach(func() {
		client = startGarden()
	})

	Describe("for many containers", func() {

		Describe(".BulkInfo", func() {
			handles := []string{"handle1", "handle2"}
			BeforeEach(func() {
				_, err := client.Create(garden.ContainerSpec{
					Handle: "handle1",
				})
				Ω(err).ShouldNot(HaveOccurred())
				_, err = client.Create(garden.ContainerSpec{
					Handle: "handle2",
				})
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("returns container info for the specified handles", func() {
				bulkInfo, err := client.BulkInfo(handles)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(bulkInfo).Should(HaveLen(2))
				for _, containerInfoEntry := range bulkInfo {
					Ω(containerInfoEntry.Err).ShouldNot(HaveOccurred())
				}
			})
		})
	})

	Describe("for a single container", func() {
		var container garden.Container

		BeforeEach(func() {
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

		Describe("info for one container", func() {
			It("includes the properties", func() {
				info, err := container.Info()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(info.Properties["foo"]).Should(Equal("bar"))
				Ω(info.Properties["a"]).Should(Equal("b"))

				Ω(info.Properties).Should(HaveLen(2))
			})
		})

		Describe("getting container metrics without getting info", func() {
			It("can list metrics", func() {
				metrics, err := container.Metrics()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(metrics).Should(BeAssignableToTypeOf(garden.Metrics{}))
				Ω(metrics).ShouldNot(Equal(garden.Metrics{}))
			})
		})

		Describe("getting container properties without getting info", func() {
			It("can list properties", func() {
				err := container.SetProperty("bar", "baz")

				value, err := container.GetProperties()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(value).Should(HaveKeyWithValue("foo", "bar"))
				Ω(value).Should(HaveKeyWithValue("bar", "baz"))
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
})
