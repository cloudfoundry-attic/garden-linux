package repository_fetcher_test

import (
	"fmt"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_container_id_provider"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_fetch_request_creator"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_remote_image_id_provider"
	"github.com/docker/docker/registry"

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

	Describe("RemoteIDProvider", func() {
		var (
			fakeRequestCreator *fake_fetch_request_creator.FakeFetchRequestCreator
			registryProviderV1 *fake_remote_image_id_provider.FakeRemoteImageIDProvider
			registryProviderV2 *fake_remote_image_id_provider.FakeRemoteImageIDProvider
			provider           *RemoteIDProvider
			apiVersion         registry.APIVersion
			fakeFetchRequest   *FetchRequest
		)

		BeforeEach(func() {
			registryProviderV1 = new(fake_remote_image_id_provider.FakeRemoteImageIDProvider)
			registryProviderV2 = new(fake_remote_image_id_provider.FakeRemoteImageIDProvider)

			fakeRequestCreator = new(fake_fetch_request_creator.FakeFetchRequestCreator)
			fakeFetchRequest = &FetchRequest{
				Endpoint: &registry.Endpoint{Version: apiVersion},
			}
			fakeRequestCreator.CreateFetchRequestReturns(fakeFetchRequest, nil)

			provider = &RemoteIDProvider{
				RequestCreator: fakeRequestCreator,
				Providers: map[registry.APIVersion]RemoteImageIDProvider{
					registry.APIVersion1: registryProviderV1,
					registry.APIVersion2: registryProviderV2,
				},
			}
		})

		Context("when request is for V1 registry", func() {
			var (
				imagePath string
				imageID   layercake.DockerImageID
			)

			BeforeEach(func() {
				apiVersion = registry.APIVersion1
				imagePath = "/path/to/image/id"
				imageID = layercake.DockerImageID("docker-image-id-1")
				registryProviderV1.ProvideImageIDReturns(imageID, nil)
			})

			It("should return image ID", func() {
				id, err := provider.ProvideID(imagePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(id).To(Equal(imageID))

				Expect(registryProviderV1.ProvideImageIDCallCount()).To(Equal(1))
				fetchRequest := registryProviderV1.ProvideImageIDArgsForCall(0)

				Expect(fetchRequest).To(Equal(fakeFetchRequest))
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
