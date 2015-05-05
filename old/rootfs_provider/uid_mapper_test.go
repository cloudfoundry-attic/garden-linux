package rootfs_provider_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider"
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
})
