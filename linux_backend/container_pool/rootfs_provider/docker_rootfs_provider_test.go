package rootfs_provider_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/fake_graph_driver"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/repository_fetcher/fake_repository_fetcher"
	. "github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/rootfs_provider"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DockerRootFSProvider", func() {
	var (
		fakeRepositoryFetcher *fake_repository_fetcher.FakeRepositoryFetcher
		fakeGraphDriver       *fake_graph_driver.FakeGraphDriver

		provider RootFSProvider
	)

	BeforeEach(func() {
		fakeRepositoryFetcher = fake_repository_fetcher.New()
		fakeGraphDriver = fake_graph_driver.New()

		provider = NewDocker(fakeRepositoryFetcher, fakeGraphDriver)
	})

	Describe("ProvideRootFS", func() {
		It("fetches it and creates a graph entry with it as the parent", func() {
			fakeRepositoryFetcher.FetchResult = "some-image-id"
			fakeGraphDriver.GetResult = "/some/graph/driver/mount/point"

			mountpoint, err := provider.ProvideRootFS("some-id", parseURL("docker:///some-repository-name"))
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeGraphDriver.Created()).To(ContainElement(
				fake_graph_driver.CreatedGraph{
					ID:     "some-id",
					Parent: "some-image-id",
				},
			))

			Expect(fakeRepositoryFetcher.Fetched()).To(ContainElement(
				fake_repository_fetcher.FetchSpec{
					Repository: "some-repository-name",
					Tag:        "latest",
				},
			))

			Expect(mountpoint).To(Equal("/some/graph/driver/mount/point"))
		})

		Context("when the url is missing a path", func() {
			It("returns an error", func() {
				_, err := provider.ProvideRootFS("some-id", parseURL("docker://"))
				Expect(err).To(Equal(ErrInvalidDockerURL))
			})
		})

		Context("and a tag is specified via a fragment", func() {
			It("uses it when fetching the repository", func() {
				_, err := provider.ProvideRootFS("some-id", parseURL("docker:///some-repository-name#some-tag"))
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRepositoryFetcher.Fetched()).To(ContainElement(
					fake_repository_fetcher.FetchSpec{
						Repository: "some-repository-name",
						Tag:        "some-tag",
					},
				))
			})
		})

		Context("but fetching it fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRepositoryFetcher.FetchError = disaster
			})

			It("returns the error", func() {
				_, err := provider.ProvideRootFS("some-id", parseURL("docker:///some-repository-name"))
				Expect(err).To(Equal(disaster))
			})
		})

		Context("but creating the graph entry fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeGraphDriver.CreateError = disaster
			})

			It("returns the error", func() {
				_, err := provider.ProvideRootFS("some-id", parseURL("docker:///some-repository-name#some-tag"))
				Expect(err).To(Equal(disaster))
			})
		})

		Context("but getting the graph entry fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeGraphDriver.GetError = disaster
			})

			It("returns the error", func() {
				_, err := provider.ProvideRootFS("some-id", parseURL("docker:///some-repository-name#some-tag"))
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("CleanupRootFS", func() {
		It("removes the container from the rootfs graph", func() {
			err := provider.CleanupRootFS("some-id")
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeGraphDriver.Putted()).To(ContainElement("some-id"))
			Expect(fakeGraphDriver.Removed()).To(ContainElement("some-id"))
		})

		Context("when removing the container from the graph fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeGraphDriver.RemoveError = disaster
			})

			It("returns the error", func() {
				err := provider.CleanupRootFS("some-id")
				Expect(err).To(Equal(disaster))
			})
		})
	})
})
