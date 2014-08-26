package rootfs_provider_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/fake_graph_driver"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/repository_fetcher/fake_repository_fetcher"
	. "github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/rootfs_provider"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DockerRootFSProvider", func() {
	var (
		fakeRepositoryFetcher *fake_repository_fetcher.FakeRepositoryFetcher
		fakeGraphDriver       *fake_graph_driver.FakeGraphDriver

		provider RootFSProvider

		logger *lagertest.TestLogger
	)

	BeforeEach(func() {
		fakeRepositoryFetcher = fake_repository_fetcher.New()
		fakeGraphDriver = fake_graph_driver.New()

		provider = NewDocker(fakeRepositoryFetcher, fakeGraphDriver)

		logger = lagertest.NewTestLogger("test")
	})

	Describe("ProvideRootFS", func() {
		It("fetches it and creates a graph entry with it as the parent", func() {
			fakeRepositoryFetcher.FetchResult = "some-image-id"
			fakeGraphDriver.GetResult = "/some/graph/driver/mount/point"

			mountpoint, envvars, err := provider.ProvideRootFS(logger, "some-id", parseURL("docker:///some-repository-name"))
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeGraphDriver.Created()).Should(ContainElement(
				fake_graph_driver.CreatedGraph{
					ID:     "some-id",
					Parent: "some-image-id",
				},
			))

			Ω(fakeRepositoryFetcher.Fetched()).Should(ContainElement(
				fake_repository_fetcher.FetchSpec{
					Repository: "some-repository-name",
					Tag:        "latest",
				},
			))

			Ω(mountpoint).Should(Equal("/some/graph/driver/mount/point"))
			Ω(envvars).Should(Equal([]string{"env1", "env1Value", "env2", "env2Value"}))
		})

		Context("when the url is missing a path", func() {
			It("returns an error", func() {
				_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("docker://"))
				Ω(err).Should(Equal(ErrInvalidDockerURL))
			})
		})

		Context("and a tag is specified via a fragment", func() {
			It("uses it when fetching the repository", func() {
				_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("docker:///some-repository-name#some-tag"))
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRepositoryFetcher.Fetched()).Should(ContainElement(
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
				_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("docker:///some-repository-name"))
				Ω(err).Should(Equal(disaster))
			})
		})

		Context("but creating the graph entry fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeGraphDriver.CreateError = disaster
			})

			It("returns the error", func() {
				_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("docker:///some-repository-name#some-tag"))
				Ω(err).Should(Equal(disaster))
			})
		})

		Context("but getting the graph entry fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeGraphDriver.GetError = disaster
			})

			It("returns the error", func() {
				_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("docker:///some-repository-name#some-tag"))
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("CleanupRootFS", func() {
		It("removes the container from the rootfs graph", func() {
			err := provider.CleanupRootFS(logger, "some-id")
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeGraphDriver.Putted()).Should(ContainElement("some-id"))
			Ω(fakeGraphDriver.Removed()).Should(ContainElement("some-id"))
		})

		Context("when removing the container from the graph fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeGraphDriver.RemoveError = disaster
			})

			It("returns the error", func() {
				err := provider.CleanupRootFS(logger, "some-id")
				Ω(err).Should(Equal(disaster))
			})
		})
	})
})
