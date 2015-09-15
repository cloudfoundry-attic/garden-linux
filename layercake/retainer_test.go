package layercake_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_id_provider"
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

	Describe("RetainByImagePath", func() {
		var idProvider *fake_id_provider.FakeIDProvider

		BeforeEach(func() {
			idProvider = new(fake_id_provider.FakeIDProvider)
			idProvider.ProvideIDReturns(layercake.DockerImageID("ABCDE"), nil)
		})

		It("retains the id of the image", func() {
			retainer.RetainByImagePath(idProvider, "some-path")
			Expect(retainer.IsHeld(layercake.DockerImageID("ABCDE"))).To(Equal(true))
		})

		Context("when the IDProvider fails", func() {
			BeforeEach(func() {
				idProvider.ProvideIDReturns(layercake.DockerImageID("ABCDE"), errors.New("IDProvider fails"))
			})

			It("does not retain the id of the image", func() {
				retainer.RetainByImagePath(idProvider, "some-path")
				Expect(retainer.IsHeld(layercake.DockerImageID("ABCDE"))).To(Equal(false))
			})

			It("returns an error", func() {
				Expect(retainer.RetainByImagePath(idProvider, "some-path")).ToNot(Succeed())
			})

		})
	})
})
