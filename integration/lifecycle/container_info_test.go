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
		handles := []string{"handle1", "handle2"}
		BeforeEach(func() {
			_, err := client.Create(garden.ContainerSpec{
				Handle: "handle1",
			})
			Expect(err).ToNot(HaveOccurred())
			_, err = client.Create(garden.ContainerSpec{
				Handle: "handle2",
			})
			Expect(err).ToNot(HaveOccurred())
		})

		Describe(".BulkInfo", func() {
			It("returns container info for the specified handles", func() {
				bulkInfo, err := client.BulkInfo(handles)
				Expect(err).ToNot(HaveOccurred())
				Expect(bulkInfo).To(HaveLen(2))
				for _, containerInfoEntry := range bulkInfo {
					Expect(containerInfoEntry.Err).ToNot(HaveOccurred())
				}
			})
		})

		Describe(".BulkMetrics", func() {
			It("returns container metrics for the specified handles", func() {
				bulkInfo, err := client.BulkMetrics(handles)
				Expect(err).ToNot(HaveOccurred())
				Expect(bulkInfo).To(HaveLen(2))
				for _, containerMetricsEntry := range bulkInfo {
					Expect(containerMetricsEntry.Err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			err := client.Destroy(container.Handle())
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("info for one container", func() {
			It("includes the properties", func() {
				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())

				Expect(info.Properties["foo"]).To(Equal("bar"))
				Expect(info.Properties["a"]).To(Equal("b"))

				Expect(info.Properties).To(HaveLen(2))
			})
		})

		Describe("getting container metrics without getting info", func() {
			It("can list metrics", func() {
				metrics, err := container.Metrics()
				Expect(err).ToNot(HaveOccurred())

				Expect(metrics).To(BeAssignableToTypeOf(garden.Metrics{}))
				Expect(metrics).ToNot(Equal(garden.Metrics{}))
			})
		})

		Describe("getting container properties without getting info", func() {
			It("can list properties", func() {
				err := container.SetProperty("bar", "baz")

				value, err := container.Properties()
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(HaveKeyWithValue("foo", "bar"))
				Expect(value).To(HaveKeyWithValue("bar", "baz"))
			})
		})

		Describe("updating container properties", func() {
			It("can CRUD", func() {
				value, err := container.Property("foo")
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal("bar"))

				err = container.SetProperty("foo", "baz")
				Expect(err).ToNot(HaveOccurred())

				err = container.RemoveProperty("a")
				Expect(err).ToNot(HaveOccurred())

				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())

				Expect(info.Properties).To(Equal(garden.Properties{
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

				Expect(err).ToNot(HaveOccurred())

				undesiredHandles = append(undesiredHandles, undesiredContainer.Handle())

				undesiredContainer, err = client.Create(garden.ContainerSpec{
					Properties: garden.Properties{
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
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("can filter by property", func() {
				containers, err := client.Containers(garden.Properties{"foo": "bar"})
				Expect(err).ToNot(HaveOccurred())

				Expect(containers).To(HaveLen(1))
				Expect(containers[0].Handle()).To(Equal(container.Handle()))

				containers, err = client.Containers(garden.Properties{"matthew": "mcconaughey"})
				Expect(err).ToNot(HaveOccurred())

				Expect(containers).To(BeEmpty())
			})
		})
	})
})
