package repository_fetcher_test

import (
	"fmt"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_container_id_provider"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RemoteMetadata", func() {
	Describe("ImageIDProvider", func() {
		var (
			provider            *ImageIDProvider
			registeredProviders map[string]ContainerIDProvider

			imagePath    string
			fakeProvider *fake_container_id_provider.FakeContainerIDProvider
		)

		BeforeEach(func() {
			registeredProviders = make(map[string]ContainerIDProvider)
		})

		JustBeforeEach(func() {
			provider = &ImageIDProvider{
				Providers: registeredProviders,
			}
		})

		BeforeEach(func() {
			fakeProvider = new(fake_container_id_provider.FakeContainerIDProvider)
			registeredProviders = map[string]ContainerIDProvider{
				"": fakeProvider,
			}
		})

		Context("when local path is used", func() {
			var (
				imageID *layercake.LocalImageID
			)

			BeforeEach(func() {
				imagePath = "/my/local/path"
				imageID = &layercake.LocalImageID{Path: "id-1", ModifiedTime: time.Now()}
				fakeProvider.ProvideIDReturns(imageID)
			})

			It("returns image ID", func() {
				id, err := provider.ProvideID(imagePath)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeProvider.ProvideIDCallCount()).To(Equal(1))

				path := fakeProvider.ProvideIDArgsForCall(0)
				Expect(path).To(Equal(imagePath))

				Expect(id).To(Equal(imageID))
			})
		})

		Context("when docker path is used", func() {
			var (
				imageID        layercake.DockerImageID
				dockerProvider *fake_container_id_provider.FakeContainerIDProvider
			)

			BeforeEach(func() {
				imagePath = "docker:///busybox"
				imageID = layercake.DockerImageID("docker-id-1")

				dockerProvider = new(fake_container_id_provider.FakeContainerIDProvider)
				dockerProvider.ProvideIDReturns(imageID)

				registeredProviders["docker"] = dockerProvider
			})

			It("returns image ID", func() {
				id, err := provider.ProvideID(imagePath)
				Expect(err).NotTo(HaveOccurred())

				Expect(dockerProvider.ProvideIDCallCount()).To(Equal(1))

				path := dockerProvider.ProvideIDArgsForCall(0)
				Expect(path).To(Equal(imagePath))

				Expect(id).To(Equal(imageID))
			})
		})

		Context("when provider does not find registered providers", func() {
			It("returns an error", func() {
				path := "docker:///i/want/this/rootfs"
				id, err := provider.ProvideID(path)
				Expect(err).To(MatchError(fmt.Sprintf("IDProvider could not be found for %s", path)))
				Expect(id).To(BeNil())
			})
		})

		Context("when invalid path is provided", func() {
			It("returns an error", func() {
				_, err := provider.ProvideID("\\+ %g\\g")
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
