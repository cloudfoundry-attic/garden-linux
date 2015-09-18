package repository_fetcher_test

import (
	"errors"
	"net/url"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_retainer"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_container_id_provider"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("ImageRetainer", func() {
	var (
		fakeGraphRetainer         *fake_retainer.FakeRetainer
		fakeRemoteImageIDProvider *fakes.FakeRemoteImageIDFetcher
		fakeDirRootfsProvider     *fake_container_id_provider.FakeContainerIDProvider

		imageRetainer *repository_fetcher.ImageRetainer
	)

	BeforeEach(func() {
		fakeGraphRetainer = new(fake_retainer.FakeRetainer)
		fakeDirRootfsProvider = new(fake_container_id_provider.FakeContainerIDProvider)
		fakeRemoteImageIDProvider = new(fakes.FakeRemoteImageIDFetcher)

		fakeDirRootfsProvider.ProvideIDStub = func(id string) layercake.ID {
			return layercake.LocalImageID{id, time.Time{}}
		}

		fakeRemoteImageIDProvider.FetchIDStub = func(id *url.URL) (layercake.ID, error) {
			return layercake.DockerImageID("/fetched/" + id.Path), nil
		}

		imageRetainer = &repository_fetcher.ImageRetainer{
			DirectoryRootfsIDProvider: fakeDirRootfsProvider,
			DockerImageIDFetcher:      fakeRemoteImageIDProvider,
			GraphRetainer:             fakeGraphRetainer,
			NamespaceCacheKey:         "chip-sandwhich",

			Logger: lagertest.NewTestLogger("test"),
		}
	})

	Context("when a single image is passed", func() {
		Context("and it is a directory rootfs", func() {
			It("retains the image", func() {
				imageRetainer.Retain([]string{
					"/foo/bar/baz",
				})

				Expect(fakeGraphRetainer.RetainCallCount()).To(Equal(2))
				Expect(fakeGraphRetainer.RetainArgsForCall(0)).To(Equal(layercake.LocalImageID{"/foo/bar/baz", time.Time{}}))
			})

			It("retains the namespaced version of the image", func() {
				imageRetainer.Retain([]string{
					"/foo/bar/baz",
				})

				Expect(fakeGraphRetainer.RetainCallCount()).To(Equal(2))
				Expect(fakeGraphRetainer.RetainArgsForCall(1)).To(Equal(
					layercake.NamespacedID(layercake.LocalImageID{"/foo/bar/baz", time.Time{}}, "chip-sandwhich"),
				))
			})
		})

		Context("and it is a docker image", func() {
			It("retains the image", func() {
				imageRetainer.Retain([]string{
					"docker://foo/bar/baz",
				})

				Expect(fakeGraphRetainer.RetainCallCount()).To(Equal(2))
				Expect(fakeGraphRetainer.RetainArgsForCall(0)).To(Equal(layercake.DockerImageID("/fetched//bar/baz")))
			})

			It("retains the namespaced version of the image", func() {
				imageRetainer.Retain([]string{
					"docker://foo/bar/baz",
				})

				Expect(fakeGraphRetainer.RetainCallCount()).To(Equal(2))
				Expect(fakeGraphRetainer.RetainArgsForCall(1)).To(Equal(
					layercake.NamespacedID(layercake.DockerImageID("/fetched//bar/baz"), "chip-sandwhich"),
				))
			})
		})
	})

	Context("when multiple images are passed", func() {
		It("retains all the images", func() {
			imageRetainer.Retain([]string{
				"docker://foo/bar/baz",
				"/foo/bar/baz",
			})

			Expect(fakeGraphRetainer.RetainCallCount()).To(Equal(4)) // both images, both namespaced version
		})

		Context("when an image id cannot be fetched", func() {
			It("still retains the other images", func() {
				fakeRemoteImageIDProvider.FetchIDStub = func(u *url.URL) (layercake.ID, error) {
					if u.Path == "/potato" {
						return nil, errors.New("boom")
					}

					return nil, nil
				}

				imageRetainer.Retain([]string{
					"docker://foo/bar/baz",
					":",
					"docker:///potato",
					"/foo/bar/baz",
				})

				Expect(fakeGraphRetainer.RetainCallCount()).To(Equal(4)) // both images, both namespaced version
			})
		})
	})
})
