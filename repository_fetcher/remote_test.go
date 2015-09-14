package repository_fetcher_test

import (
	"errors"
	"net/url"

	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_fetch_request_creator"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_versioned_fetcher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RepositoryFetcher", func() {
	var (
		fakeRequestCreator *fake_fetch_request_creator.FakeFetchRequestCreator
		fetcher            RepositoryFetcher
		logger             *lagertest.TestLogger

		v1Fetcher *fake_versioned_fetcher.FakeVersionedFetcher
		v2Fetcher *fake_versioned_fetcher.FakeVersionedFetcher

		returnedSession  *registry.Session
		returnedEndpoint *registry.Endpoint
		apiversion       registry.APIVersion
	)

	BeforeEach(func() {
		fakeRequestCreator = new(fake_fetch_request_creator.FakeFetchRequestCreator)
		v1Fetcher = new(fake_versioned_fetcher.FakeVersionedFetcher)
		v1Fetcher.FetchReturns(&FetchResponse{ImageID: "some-image-id"}, nil)

		v2Fetcher = new(fake_versioned_fetcher.FakeVersionedFetcher)
		fetchers := map[registry.APIVersion]VersionedFetcher{
			registry.APIVersion1: v1Fetcher,
			registry.APIVersion2: v2Fetcher,
		}

		fetcher = NewRemote(fakeRequestCreator, fetchers)
		logger = lagertest.NewTestLogger("test")
	})

	JustBeforeEach(func() {
		returnedSession = &registry.Session{}
		returnedEndpoint = &registry.Endpoint{Version: apiversion}

		fakeRequestCreator.CreateFetchRequestStub = func(logger lager.Logger, repoURL *url.URL, diskQuota int64) (*FetchRequest, error) {
			return &FetchRequest{
				Session:    returnedSession,
				Endpoint:   returnedEndpoint,
				Path:       repoURL.Path,
				RemotePath: repoURL.Path,
				Tag:        repoURL.Fragment,
				MaxSize:    diskQuota,
			}, nil
		}
	})

	Describe("Fetch", func() {
		Describe("create a correct fetch request", func() {
			It("creates a fetch request to the registry provider based on the host and port of the repo url", func() {
				parsedURL, err := url.Parse("some-scheme://some-registry:4444/some-repo#some-tag")
				Expect(err).ToNot(HaveOccurred())

				fetcher.Fetch(logger, parsedURL, 0)

				Expect(fakeRequestCreator.CreateFetchRequestCallCount()).To(Equal(1))
				log, imageUrl, imageQuota := fakeRequestCreator.CreateFetchRequestArgsForCall(0)
				Expect(log).To(Equal(logger))
				Expect(imageUrl).To(Equal(parsedURL))
				Expect(imageQuota).To(Equal(int64(0)))
			})

			Context("when retrieving a session from the registry provider errors", func() {
				JustBeforeEach(func() {
					fakeRequestCreator.CreateFetchRequestReturns(nil, errors.New("oh no"))
				})

				It("returns the error, suitably wrapped", func() {
					parsedURL, err := url.Parse("some-scheme://some-registry:4444/some-repo#some-tag")
					Expect(err).ToNot(HaveOccurred())

					_, _, _, err = fetcher.Fetch(logger, parsedURL, 0)
					Expect(err).To(MatchError(ContainSubstring("oh no")))
				})
			})
		})

		Describe("Fetching", func() {
			Context("when the version is known", func() {
				BeforeEach(func() {
					apiversion = registry.APIVersion1
				})

				It("uses the correct fetcher to fetch", func() {
					imageID, _, _, _ := fetcher.Fetch(logger, &url.URL{Path: "/foo/somePath", Fragment: "someTag"}, 987)
					Expect(imageID).To(Equal("some-image-id"))

					Expect(v1Fetcher.FetchCallCount()).To(Equal(1))

					fetchRequest := v1Fetcher.FetchArgsForCall(0)

					Expect(fetchRequest.Path).To(Equal("/foo/somePath"))
					Expect(fetchRequest.RemotePath).To(Equal("/foo/somePath"))
					Expect(fetchRequest.Tag).To(Equal("someTag"))
					Expect(fetchRequest.Session).To(Equal(returnedSession))
					Expect(fetchRequest.Endpoint).To(Equal(returnedEndpoint))
					Expect(fetchRequest.MaxSize).To(Equal(int64(987)))
				})

				It("does not call the other fetcher", func() {
					fetcher.Fetch(logger, &url.URL{Path: "/foo/somePath"}, 0)
					Expect(v2Fetcher.FetchCallCount()).To(Equal(0))
				})
			})

			Context("When the version is unknown", func() {
				BeforeEach(func() {
					apiversion = registry.APIVersion(42)
				})

				It("totally throws an error", func() {
					_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: "/bar"}, 0)
					Expect(err).To(MatchError("unknown docker registry API version"))
				})
			})
		})
	})
})
