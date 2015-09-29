package repository_fetcher_test

import (
	"net/url"

	. "github.com/cloudfoundry-incubator/garden-linux/shed/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/shed/repository_fetcher/fake_fetch_request_creator"
	"github.com/cloudfoundry-incubator/garden-linux/shed/repository_fetcher/fake_versioned_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/shed/repository_fetcher/fakes"
	"github.com/docker/docker/registry"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FetcherFactory", func() {
	var (
		fakeV1Fetcher, fakeV2Fetcher *fake_versioned_fetcher.FakeVersionedFetcher
		fakeLocalFetcher             *fakes.FakeRepositoryFetcher
		fakeRequestCreator           *fake_fetch_request_creator.FakeFetchRequestCreator
		factory                      *CompositeFetcher
		req                          *FetchRequest
	)

	BeforeEach(func() {
		fakeV1Fetcher = new(fake_versioned_fetcher.FakeVersionedFetcher)
		fakeV2Fetcher = new(fake_versioned_fetcher.FakeVersionedFetcher)
		fakeLocalFetcher = new(fakes.FakeRepositoryFetcher)
		fakeRequestCreator = new(fake_fetch_request_creator.FakeFetchRequestCreator)

		factory = &CompositeFetcher{
			RequestCreator: fakeRequestCreator,
			LocalFetcher:   fakeLocalFetcher,
			RemoteFetchers: map[registry.APIVersion]VersionedFetcher{
				registry.APIVersion1: fakeV1Fetcher,
				registry.APIVersion2: fakeV2Fetcher,
			},
		}
	})

	Context("when the URL does not contain a scheme", func() {
		It("delegates .Fetch to the local fetcher", func() {
			factory.Fetch(&url.URL{Path: "cake"}, 24)
			Expect(fakeLocalFetcher.FetchCallCount()).To(Equal(1))
		})

		It("delegates .FetchID to the local fetcher", func() {
			factory.FetchID(&url.URL{Path: "cake"})
			Expect(fakeLocalFetcher.FetchIDCallCount()).To(Equal(1))
		})
	})

	Context("with a V1 endpoint", func() {
		BeforeEach(func() {
			req = &FetchRequest{
				Endpoint: &registry.Endpoint{Version: registry.APIVersion1},
			}

			fakeRequestCreator.CreateFetchRequestReturns(req, nil)
		})

		It("passes the arguments to the request creator", func() {
			factory.Fetch(&url.URL{Scheme: "docker", Path: "cake"}, 24)
			Expect(fakeRequestCreator.CreateFetchRequestCallCount()).To(Equal(1))

			url, quota := fakeRequestCreator.CreateFetchRequestArgsForCall(0)
			Expect(url.Path).To(Equal("cake"))
			Expect(quota).To(BeEquivalentTo(24))
		})

		It("uses a v1 image fetcher", func() {
			factory.Fetch(&url.URL{Scheme: "docker"}, 12)
			Expect(fakeV1Fetcher.FetchCallCount()).To(Equal(1))
			Expect(fakeV1Fetcher.FetchArgsForCall(0)).To(Equal(req))
		})

		It("uses a v1 image id fetcher", func() {
			req := &FetchRequest{
				Endpoint: &registry.Endpoint{Version: registry.APIVersion1},
			}

			factory.FetchID(&url.URL{Scheme: "docker"})
			Expect(fakeV1Fetcher.FetchIDCallCount()).To(Equal(1))
			Expect(fakeV1Fetcher.FetchIDArgsForCall(0)).To(Equal(req))
		})
	})

	Context("with a V2 endpoint", func() {
		BeforeEach(func() {
			req = &FetchRequest{
				Endpoint: &registry.Endpoint{Version: registry.APIVersion2},
			}

			fakeRequestCreator.CreateFetchRequestReturns(req, nil)
		})

		It("uses a v2 image fetcher", func() {
			factory.Fetch(&url.URL{Scheme: "docker"}, 12)
			Expect(fakeV2Fetcher.FetchCallCount()).To(Equal(1))
			Expect(fakeV2Fetcher.FetchArgsForCall(0)).To(Equal(req))
		})
	})

	Context("with an unknown api version", func() {
		BeforeEach(func() {
			req = &FetchRequest{
				Endpoint: &registry.Endpoint{Version: registry.APIVersionUnknown},
			}

			fakeRequestCreator.CreateFetchRequestReturns(req, nil)
		})

		It("returns an error while fetching an image", func() {
			_, err := factory.Fetch(&url.URL{Scheme: "docker"}, 12)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error while fecthing an image id", func() {
			_, err := factory.FetchID(&url.URL{Scheme: "docker"})
			Expect(err).To(HaveOccurred())
		})
	})
})
