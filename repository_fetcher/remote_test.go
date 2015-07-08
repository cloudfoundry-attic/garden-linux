package repository_fetcher_test

import (
	"errors"
	"net/url"

	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/resource_pool/fake_graph"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RepositoryFetcher", func() {
	var graph *fake_graph.FakeGraph
	var fetcher RepositoryFetcher
	var logger *lagertest.TestLogger
	var fakeRegistryProvider *fakes.FakeRegistryProvider

	BeforeEach(func() {
		graph = fake_graph.New()

		fakeRegistryProvider = new(fakes.FakeRegistryProvider)
		fakeRegistryProvider.ApplyDefaultHostnameReturns("some-repo")
		fakeRegistryProvider.ProvideRegistryReturns(nil, nil, errors.New("Session and endpoint not provided"))
		fetcher = NewRemote(fakeRegistryProvider, graph)

		logger = lagertest.NewTestLogger("test")
	})

	Describe("Fetch", func() {
		Context("when the path is empty", func() {
			It("returns an error", func() {
				_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: ""}, "some-tag")
				Expect(err).To(Equal(ErrInvalidDockerURL))
			})
		})

		Describe("connecting to the correct registry", func() {
			BeforeEach(func() {
				fakeRegistryProvider.ApplyDefaultHostnameReturns("some-registry:4444")
			})

			It("retrieves the registry from the registry provider based on the host and port of the repo url", func() {
				parsedURL, err := url.Parse("some-scheme://some-registry:4444/some-repo")
				Expect(err).ToNot(HaveOccurred())

				fetcher.Fetch(logger, parsedURL, "some-tag")

				Expect(fakeRegistryProvider.ApplyDefaultHostnameCallCount()).To(Equal(1))
				Expect(fakeRegistryProvider.ApplyDefaultHostnameArgsForCall(0)).To(Equal("some-registry:4444"))

				Expect(fakeRegistryProvider.ProvideRegistryCallCount()).To(Equal(1))
				Expect(fakeRegistryProvider.ProvideRegistryArgsForCall(0)).To(Equal("some-registry:4444"))
			})

			Context("when retrieving a session from the registry provider errors", func() {
				It("returns the error, suitably wrapped", func() {
					parsedURL, err := url.Parse("some-scheme://some-registry:4444/some-repo")
					Expect(err).ToNot(HaveOccurred())

					_, _, _, err = fetcher.Fetch(logger, parsedURL, "some-tag")
					Expect(err).To(MatchError(ContainSubstring("repository_fetcher: ProvideRegistry: could not fetch image some-repo from registry some-registry:4444:")))
				})
			})
		})
	})
})
