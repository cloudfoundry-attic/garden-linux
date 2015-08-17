package rootfs_provider_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MappingList", func() {
	Context("when the mapping does not contain the given id", func() {
		It("returns the original id", func() {
			mapping := rootfs_provider.MappingList{}
			Expect(mapping.Map(55)).To(Equal(55))
		})
	})

	Context("when the mapping contains the given id but the range size is zero", func() {
		It("returns the original id", func() {
			mapping := rootfs_provider.MappingList{{
				FromID: 55,
				ToID:   77,
				Size:   0,
			}}

			Expect(mapping.Map(55)).To(Equal(55))
		})
	})

	Context("when the mapping contains the given id as the first element of a range", func() {
		It("returns the mapped id", func() {
			mapping := rootfs_provider.MappingList{{
				FromID: 55,
				ToID:   77,
				Size:   1,
			}}

			Expect(mapping.Map(55)).To(Equal(77))
		})
	})

	Context("when the mapping contains the given id as path of a range", func() {
		It("returns the mapped id", func() {
			mapping := rootfs_provider.MappingList{{
				FromID: 55,
				ToID:   77,
				Size:   10,
			}}

			Expect(mapping.Map(64)).To(Equal(86))
		})
	})

	Context("when the uid is just outside of the range of a mapping (defensive)", func() {
		It("returns the original id", func() {
			mapping := rootfs_provider.MappingList{{
				FromID: 55,
				ToID:   77,
				Size:   10,
			}}

			Expect(mapping.Map(65)).To(Equal(65))
		})
	})

	Describe("String", func() {
		Context("when the mapping is empty", func() {
			It("returns the string 'empty'", func() {
				mapping := rootfs_provider.MappingList{}
				Expect(mapping.String()).To(Equal("empty"))
			})
		})

		Context("when the mapping has a single entry", func() {
			It("returns a valid representation", func() {
				mapping := rootfs_provider.MappingList{
					rootfs_provider.Mapping{
						FromID: 122,
						ToID:   123456,
						Size:   125000,
					},
				}

				Expect(mapping.String()).To(Equal("122-123456-125000"))
			})
		})

		Context("when the mapping has multiple entries", func() {
			It("returns a valid representation containing all the entries", func() {
				mapping := rootfs_provider.MappingList{
					{
						FromID: 1,
						ToID:   2,
						Size:   3,
					},
					{
						FromID: 4,
						ToID:   5,
						Size:   6,
					},
				}

				Expect(mapping.String()).To(Equal("1-2-3,4-5-6"))
			})
		})
	})

})
