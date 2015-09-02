package layercake_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("retainer", func() {
	var (
		retainer layercake.Retainer
		id       layercake.ID
	)

	BeforeEach(func() {
		retainer = layercake.NewRetainer()
		id = layercake.DockerImageID("banana")
	})

	Describe("IsHeld", func() {
		Context("when the image is not retained", func() {
			It("should not be held", func() {
				Expect(retainer.IsHeld(id)).To(BeFalse())
			})
		})

		Context("when the image is retained", func() {
			BeforeEach(func() {
				retainer.Retain(id)
			})

			It("should be held", func() {
				Expect(retainer.IsHeld(id)).To(BeTrue())
			})

			Context("and the image is released", func() {
				BeforeEach(func() {
					retainer.Release(id)
				})

				It("should not be held", func() {
					Expect(retainer.IsHeld(id)).To(BeFalse())
				})
			})
		})

		Context("when the image is retained twice", func() {
			BeforeEach(func() {
				retainer.Retain(id)
				retainer.Retain(id)
			})

			Context("and released once", func() {
				BeforeEach(func() {
					retainer.Release(id)
				})

				It("should still be held", func() {
					Expect(retainer.IsHeld(id)).To(BeTrue())
				})
			})
		})
	})
})
