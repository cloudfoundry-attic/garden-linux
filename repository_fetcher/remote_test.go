package repository_fetcher_test

import (
	"errors"
	"net/url"

	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_pinger"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_versioned_fetcher"
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

	var v1Fetcher *fake_versioned_fetcher.FakeVersionedFetcher
	var v2Fetcher *fake_versioned_fetcher.FakeVersionedFetcher
	var pinger *fake_pinger.FakePinger

	BeforeEach(func() {
		graph = fake_graph.New()

		v1Fetcher = new(fake_versioned_fetcher.FakeVersionedFetcher)
		v2Fetcher = new(fake_versioned_fetcher.FakeVersionedFetcher)
		pinger = new(fake_pinger.FakePinger)

		fakeRegistryProvider = new(fakes.FakeRegistryProvider)
		fakeRegistryProvider.ProvideRegistryReturns(nil, nil, errors.New("Session and endpoint not provided"))
		fetchers := map[registry.APIVersion]VersionedFetcher{
			registry.APIVersion1: v1Fetcher,
			registry.APIVersion2: v2Fetcher,
		}

		fetcher = NewRemote(fakeRegistryProvider, graph, fetchers, pinger)
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
			It("retrieves the registry from the registry provider based on the host and port of the repo url", func() {
				parsedURL, err := url.Parse("some-scheme://some-registry:4444/some-repo")
				Expect(err).ToNot(HaveOccurred())

				fetcher.Fetch(logger, parsedURL, "some-tag")

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

		Describe("Fetching", func() {
			var (
				returnedSession  *registry.Session
				returnedEndpoint *registry.Endpoint

				apiversion registry.APIVersion
			)

			JustBeforeEach(func() {
				returnedSession = &registry.Session{}
				returnedEndpoint = &registry.Endpoint{Version: apiversion}
				fakeRegistryProvider.ProvideRegistryStub = func(hostname string) (*registry.Session, *registry.Endpoint, error) {
					return returnedSession, returnedEndpoint, nil
				}

				v1Fetcher.FetchReturns(&FetchResponse{ImageID: "some-image-id"}, nil)
			})

			Context("when the version is known", func() {
				BeforeEach(func() {
					apiversion = registry.APIVersion1
				})

				It("uses the correct fetcher to fetch", func() {
					imageID, _, _, _ := fetcher.Fetch(logger, &url.URL{Path: "/foo/somePath"}, "someTag")
					Expect(imageID).To(Equal("some-image-id"))

					Expect(v1Fetcher.FetchCallCount()).To(Equal(1))
					Expect(v1Fetcher.FetchArgsForCall(0).Path).To(Equal("foo/somePath"))
					Expect(v1Fetcher.FetchArgsForCall(0).RemotePath).To(Equal("foo/somePath"))
					Expect(v1Fetcher.FetchArgsForCall(0).Tag).To(Equal("someTag"))
					Expect(v1Fetcher.FetchArgsForCall(0).Session).To(Equal(returnedSession))
					Expect(v1Fetcher.FetchArgsForCall(0).Endpoint).To(Equal(returnedEndpoint))
				})

				It("does not call the other fetcher", func() {
					fetcher.Fetch(logger, &url.URL{Path: "/foo/somePath"}, "someTag")
					Expect(v2Fetcher.FetchCallCount()).To(Equal(0))
				})

				Context("when the endpoint is not standalone", func() {
					It("prepends library prefore the remote path if the path does not contain a /", func() {
						fetcher.Fetch(logger, &url.URL{Path: "/somePath"}, "someTag")
						Expect(v1Fetcher.FetchArgsForCall(0).RemotePath).To(Equal("library/somePath"))
					})
				})

				Context("when the endpoint IS standalone", func() {
					BeforeEach(func() {
						pinger.PingReturns(registry.RegistryInfo{
							Standalone: true,
						}, nil)
					})

					It("does not prepend library/ prefore the remote path if the path does not contain a /", func() {
						fetcher.Fetch(logger, &url.URL{Path: "/somePath"}, "someTag")
						Expect(v1Fetcher.FetchArgsForCall(0).RemotePath).To(Equal("somePath"))
					})
				})
			})

			Context("When the version is unknown", func() {
				BeforeEach(func() {
					apiversion = registry.APIVersion(42)
				})

				It("totally throws an error", func() {
					_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: "/bar"}, "tag")
					Expect(err).To(MatchError("unknown docker registry API version"))
				})
			})
		})
	})
})
