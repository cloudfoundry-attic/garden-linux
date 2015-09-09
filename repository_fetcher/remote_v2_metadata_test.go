package repository_fetcher_test

import (
	"net/http"

	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("RemoteV2Metadata", func() {
	var (
		server       *ghttp.Server
		logger       *lagertest.TestLogger
		provider     *ImageV2MetadataProvider
		fetchRequest *FetchRequest
		registryAddr string
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		server, registryAddr, fetchRequest = createFakeHTTPV2RegistryServer(logger)
		provider = &ImageV2MetadataProvider{}
	})

	Context("when fetching manifest fails", func() {
		Context("when the manifest endpoint fails", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v2/some-repo/manifests/some-tag"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.WriteHeader(500)
						}),
					),
				)
			})

			It("returns an error", func() {
				_, err := provider.ProvideMetadata(fetchRequest)
				Expect(err).To(MatchError(ContainSubstring("repository_fetcher: GetV2ImageManifest: could not fetch image some-repo from registry %s:", registryAddr)))
			})
		})

		Context("when the provided manifest is invalid", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v2/some-repo/manifests/some-tag"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Write([]byte(`{\"hello}`))
						}),
					),
				)
			})

			It("returns an error", func() {
				_, err := provider.ProvideMetadata(fetchRequest)
				Expect(err).To(MatchError(ContainSubstring("repository_fetcher: UnmarshalManifest: could not fetch image some-repo from registry %s:", registryAddr)))
			})
		})
	})

	It("fetch the provided metadata", func() {
		setupSuccessfulV2Fetch(server, false)

		metadata, err := provider.ProvideMetadata(fetchRequest)
		Expect(err).NotTo(HaveOccurred())

		Expect(metadata.Images).To(HaveLen(2))
		Expect(metadata.Images[0].ID).To(Equal("banana-pie-2"))
		Expect(metadata.Images[1].ID).To(Equal("banana-pie-1"))
	})

	Describe("FetchProvideID", func() {
		Context("when succeeds", func() {
			BeforeEach(func() {
				setupSuccessfulV2Fetch(server, false)
			})

			It("returns image ID", func() {
				imgID, err := provider.ProvideImageID(fetchRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(imgID).To(Equal("banana-pie-2"))
			})
		})

		Context("when fails to fetch image id", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v2/some-repo/manifests/some-tag"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.WriteHeader(500)
						}),
					),
				)
			})

			It("should return an error", func() {
				_, err := provider.ProvideImageID(fetchRequest)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Server error: 500 trying to fetch for some-repo:some-tag"))
			})
		})
	})
})
