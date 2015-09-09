package repository_fetcher_test

import (
	"fmt"
	"net/http"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("RemoteV1Metadata", func() {
	var (
		server          *ghttp.Server
		endpoint1Server *ghttp.Server
		endpoint2Server *ghttp.Server
		provider        *ImageV1MetadataProvider
		fetchRequest    *FetchRequest
		registryAddr    string
	)

	BeforeEach(func() {
		logger := lagertest.NewTestLogger("test")
		server, endpoint1Server, endpoint2Server, registryAddr, fetchRequest = createFakeHTTPV1RegistryServer(logger)
		provider = &ImageV1MetadataProvider{}
	})

	It("should return metadata", func() {
		metadata, err := provider.ProvideMetadata(fetchRequest)
		Expect(err).NotTo(HaveOccurred())

		Expect(metadata.ImageID).To(Equal("id-1"))
		Expect(metadata.Endpoints).To(Equal([]string{
			fmt.Sprintf("http://%s/v1/", endpoint1Server.HTTPTestServer.Listener.Addr().String()),
			fmt.Sprintf("http://%s/v1/", endpoint2Server.HTTPTestServer.Listener.Addr().String()),
		}))
	})

	Context("when fetching repository data fails", func() {
		BeforeEach(func() {
			server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(500)
			}))
		})

		It("returns an error", func() {
			_, err := provider.ProvideMetadata(fetchRequest)
			Expect(err).To(MatchError(ContainSubstring("repository_fetcher: GetRepositoryData: could not fetch image some-repo from registry %s:", registryAddr)))
		})
	})

	Context("when fetching the remote tags fails", func() {
		BeforeEach(func() {
			endpoint1Server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(500)
			}))

			endpoint2Server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/v1/repositories/library/some-repo/tags"),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Write([]byte(`{
							"some-tag": "id-1",
							"some-other-tag": "id-2"
						}`))
					}),
				),
			)

			setupSuccessfulFetch(endpoint1Server)
		})

		Context("on all endpoints", func() {
			BeforeEach(func() {
				endpoint2Server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(500)
				}))
			})

			It("returns an error", func() {
				_, err := provider.ProvideMetadata(fetchRequest)
				Expect(err).To(MatchError(ContainSubstring("repository_fetcher: GetRemoteTags: could not fetch image some-repo from registry %s:", registryAddr)))
			})
		})
	})

	Describe("ProvideImageID", func() {
		It("returns image ID", func() {
			imgID, err := provider.ProvideImageID(fetchRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(imgID).To(Equal(layercake.DockerImageID("id-1")))
		})

		Context("when fails to fetch image id", func() {
			BeforeEach(func() {
				server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(500)
				}))
			})

			It("should return an error", func() {
				_, err := provider.ProvideImageID(fetchRequest)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Status 500 trying to pull repository some-repo"))
			})
		})
	})
})
