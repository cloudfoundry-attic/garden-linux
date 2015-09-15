package repository_fetcher_test

import (
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_retainer"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_container_id_provider"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

		fakeRemoteImageIDProvider.FetchIDStub = func(id string) (layercake.ID, error) {
			return layercake.DockerImageID("/fetched/" + id), nil
		}

		imageRetainer = &repository_fetcher.ImageRetainer{
			DirectoryRootfsIDProvider: fakeDirRootfsProvider,
			DockerImageIDFetcher:      fakeRemoteImageIDProvider,
			GraphRetainer:             fakeGraphRetainer,
		}
	})

	Context("when a single image is passed", func() {
		Context("and it is a directory rootfs", func() {
			It("retains the image", func() {
				imageRetainer.Retain([]string{
					"/foo/bar/baz",
				})

				Expect(fakeGraphRetainer.RetainCallCount()).To(Equal(1))
				Expect(fakeGraphRetainer.RetainArgsForCall(0)).To(Equal(layercake.LocalImageID{"/foo/bar/baz", time.Time{}}))
			})
		})

		Context("and it is a docker image", func() {
			It("retains the image", func() {
				imageRetainer.Retain([]string{
					"docker://foo/bar/baz",
				})

				Expect(fakeGraphRetainer.RetainCallCount()).To(Equal(1))
				Expect(fakeGraphRetainer.RetainArgsForCall(0)).To(Equal(layercake.DockerImageID("/fetched/docker://foo/bar/baz")))
			})
		})
	})
})
