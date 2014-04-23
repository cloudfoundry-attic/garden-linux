package lifecycle_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.google.com/p/gogoprotobuf/proto"
	warden "github.com/cloudfoundry-incubator/garden/protocol"
)

var _ = Describe("A container with properties", func() {
	var handle string

	BeforeEach(func() {
		res, err := client.Create(map[string]string{
			"foo": "bar",
			"a":   "b",
		})

		Expect(err).ToNot(HaveOccurred())

		handle = res.GetHandle()
	})

	AfterEach(func() {
		_, err := client.Destroy(handle)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("when reporting the container's info", func() {
		It("includes the properties", func() {
			info, err := client.Info(handle)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.GetProperties()).Should(ContainElement(&warden.Property{
				Key:   proto.String("foo"),
				Value: proto.String("bar"),
			}))

			Ω(info.GetProperties()).Should(ContainElement(&warden.Property{
				Key:   proto.String("a"),
				Value: proto.String("b"),
			}))

			Ω(info.GetProperties()).Should(HaveLen(2))
		})
	})

	Describe("when listing container info", func() {
		var undesiredHandles []string
		BeforeEach(func() {
			res, err := client.Create(map[string]string{
				"foo": "baz",
				"a":   "b",
			})

			Expect(err).ToNot(HaveOccurred())
			undesiredHandles = append(undesiredHandles, res.GetHandle())

			res, err = client.Create(map[string]string{
				"baz": "bar",
				"a":   "b",
			})

			Expect(err).ToNot(HaveOccurred())

			undesiredHandles = append(undesiredHandles, res.GetHandle())
		})

		AfterEach(func() {
			for _, handle := range undesiredHandles {
				_, err := client.Destroy(handle)
				Ω(err).ShouldNot(HaveOccurred())
			}
		})

		It("can filter by property", func() {
			res, err := client.List(map[string]string{"foo": "bar"})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(res.GetHandles()).Should(Equal([]string{handle}))

			res, err = client.List(map[string]string{"matthew": "mcconaughey"})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(res.GetHandles()).Should(BeEmpty())
		})
	})
})
