package repository_fetcher_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry-incubator/garden-linux/old/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/old/repository_fetcher/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/resource_pool/fake_graph"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("RepositoryFetcher", func() {
	var graph *fake_graph.FakeGraph
	var fetcher RepositoryFetcher

	var logger *lagertest.TestLogger

	var server *ghttp.Server
	var endpoint1 *ghttp.Server
	var endpoint2 *ghttp.Server

	var fakeRegistryProvider *fakes.FakeRegistryProvider

	BeforeEach(func() {
		graph = fake_graph.New()

		server = ghttp.NewServer()

		endpoint1 = ghttp.NewServer()
		endpoint2 = ghttp.NewServer()

		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/_ping"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("X-Docker-Registry-Version", "v1")
					w.Header().Add("X-Docker-Registry-Standalone", "true")
					w.Write([]byte(`{"standalone": true, "version": "v1"}`))
				}),
			),
		)

		endpoint, err := registry.NewEndpoint(
			server.HTTPTestServer.Listener.Addr().String(),
			[]string{server.HTTPTestServer.Listener.Addr().String()},
		)
		Expect(err).ToNot(HaveOccurred())

		registry, err := registry.NewSession(nil, nil, endpoint, true)
		Expect(err).ToNot(HaveOccurred())

		fakeRegistryProvider = new(fakes.FakeRegistryProvider)
		fakeRegistryProvider.ApplyDefaultHostnameReturns("some-repo")
		fakeRegistryProvider.ProvideRegistryReturns(registry, nil)
		fetcher = NewRemote(fakeRegistryProvider, graph)

		logger = lagertest.NewTestLogger("test")
	})

	setupSuccessfulFetch := func(endpoint *ghttp.Server) {
		endpoint.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/images/layer-3/json"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Add("X-Docker-Size", "123")
					w.Write([]byte(`{"id":"layer-3","parent":"parent-3","Config":{"env": ["env2=env2Value", "malformedenvvar"]}}`))
				}),
			),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/images/layer-3/layer"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Write([]byte(`layer-3-data`))
				}),
			),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/images/layer-2/json"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Add("X-Docker-Size", "456")
					w.Write([]byte(`{"id":"layer-2","parent":"parent-2","Config":{"volumes": { "/tmp": {}, "/another": {} }, "env": ["env1=env1Value", "env2=env2NewValue"]}}`))
				}),
			),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/images/layer-2/layer"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Write([]byte(`layer-2-data`))
				}),
			),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/images/layer-1/json"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Add("X-Docker-Size", "789")
					w.Write([]byte(`{"id":"layer-1","parent":"parent-1"}`))
				}),
			),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/images/layer-1/layer"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Write([]byte(`layer-1-data`))
				}),
			),
		)
	}

	Describe("Fetch", func() {
		BeforeEach(func() {
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/v1/repositories/some-repo/images"),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Header().Set("X-Docker-Token", "token-1,token-2")
						w.Header().Add("X-Docker-Endpoints", endpoint1.HTTPTestServer.Listener.Addr().String())
						w.Header().Add("X-Docker-Endpoints", endpoint2.HTTPTestServer.Listener.Addr().String())
						w.Write([]byte(`[
							{"id": "id-1", "checksum": "sha-1"},
							{"id": "id-2", "checksum": "sha-2"}
						]`))
					}),
				),
			)

			endpoint1.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/v1/repositories/library/some-repo/tags"),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Write([]byte(`{
							"some-tag": "id-1",
							"some-other-tag": "id-2"
						}`))
					}),
				),
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/v1/images/id-1/ancestry"),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Write([]byte(`["layer-1", "layer-2", "layer-3"]`))
					}),
				),
			)
		})

		Context("when the path is empty", func() {
			It("returns an error", func() {
				_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: ""}, "some-tag")
				Expect(err).To(Equal(ErrInvalidDockerURL))
			})
		})

		Describe("connecting to the correct registry", func() {
			BeforeEach(func() {
				setupSuccessfulFetch(endpoint1)
				fakeRegistryProvider.ApplyDefaultHostnameReturns("some-registry:4444")
			})

			It("retrieves the registry from the registry provider based on the host and port of the repo url", func() {
				fetcher.Fetch(logger, parseURL("some-scheme://some-registry:4444/some-repo"), "some-tag")

				Expect(fakeRegistryProvider.ApplyDefaultHostnameCallCount()).To(Equal(1))
				Expect(fakeRegistryProvider.ApplyDefaultHostnameArgsForCall(0)).To(Equal("some-registry:4444"))

				Expect(fakeRegistryProvider.ProvideRegistryCallCount()).To(Equal(1))
				Expect(fakeRegistryProvider.ProvideRegistryArgsForCall(0)).To(Equal("some-registry:4444"))
			})

			Context("when retrieving a session from the registry provider errors", func() {
				It("returns the error, suitably wrapped", func() {
					fakeRegistryProvider.ProvideRegistryReturns(nil, errors.New("an error"))

					_, _, _, err := fetcher.Fetch(logger, parseURL("some-scheme://some-registry:4444/some-repo"), "some-tag")
					Expect(err).To(MatchError("repository_fetcher: ProvideRegistry: could not fetch image some-repo from registry some-registry:4444: an error"))
				})
			})
		})

		Context("when none of the layers already exist", func() {
			BeforeEach(func() {
				setupSuccessfulFetch(endpoint1)
			})

			It("downloads all layers of the given tag of a repository and returns its image id", func() {
				expectedLayerNum := 3

				graph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
					Expect(image.ID).To(Equal(fmt.Sprintf("layer-%d", expectedLayerNum)))
					Expect(image.Parent).To(Equal(fmt.Sprintf("parent-%d", expectedLayerNum)))

					layerData, err := ioutil.ReadAll(layer)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(layerData)).To(Equal(fmt.Sprintf("layer-%d-data", expectedLayerNum)))

					expectedLayerNum--

					return nil
				}

				imageID, envvars, volumes, err := fetcher.Fetch(
					logger,
					parseURL("scheme://host/some-repo"),
					"some-tag",
				)

				Expect(err).ToNot(HaveOccurred())
				Expect(envvars).To(Equal(process.Env{"env1": "env1Value", "env2": "env2NewValue"}))
				Expect(volumes).To(ConsistOf([]string{"/tmp", "/another"}))
				Expect(imageID).To(Equal("id-1"))
			})

			Context("when the first endpoint fails", func() {
				BeforeEach(func() {
					endpoint1.SetHandler(1, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(500)
					}))

					endpoint2.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/v1/images/id-1/ancestry"),
							http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
								w.Write([]byte(`["layer-1", "layer-2", "layer-3"]`))
							}),
						),
					)

					setupSuccessfulFetch(endpoint2)
				})

				It("retries with the next endpoint", func() {
					imageID, _, _, err := fetcher.Fetch(
						logger,
						parseURL("scheme://host/some-repo"),
						"some-tag",
					)
					Expect(err).ToNot(HaveOccurred())

					Expect(imageID).To(Equal("id-1"))
				})

				Context("and the rest also fail", func() {
					BeforeEach(func() {
						endpoint2.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.WriteHeader(500)
						}))
					})

					It("returns an error", func() {
						_, _, _, err := fetcher.Fetch(
							logger,
							parseURL("scheme://host/some-repo"),
							"some-tag",
						)
						Expect(err.Error()).To(ContainSubstring("repository_fetcher: fetchFromEndPoint: could not fetch image some-repo from registry some-repo: all endpoints failed:"))
					})
				})
			})
		})

		Context("when an image already exists in the graph", func() {
			BeforeEach(func() {
				graph.SetExists("layer-2", []byte(`{"id":"layer-2","parent":"parent-2","Config":{"env": ["env2=env2Value"]}}`))

				endpoint1.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/layer-3/json"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Header().Add("X-Docker-Size", "123")
							w.Write([]byte(`{"id":"layer-3","parent":"parent-3"}`))
						}),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/layer-3/layer"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Write([]byte(`layer-3-data`))
						}),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/layer-1/json"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Header().Add("X-Docker-Size", "789")
							w.Write([]byte(`{"id":"layer-1","parent":"parent-1"}`))
						}),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/layer-1/layer"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Write([]byte(`layer-1-data`))
						}),
					),
				)
			})

			It("does not fetch it", func() {
				expectedLayerNum := 3

				graph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
					Expect(image.ID).To(Equal(fmt.Sprintf("layer-%d", expectedLayerNum)))
					Expect(image.Parent).To(Equal(fmt.Sprintf("parent-%d", expectedLayerNum)))

					layerData, err := ioutil.ReadAll(layer)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(layerData)).To(Equal(fmt.Sprintf("layer-%d-data", expectedLayerNum)))

					expectedLayerNum--

					// skip 2 as it already exists as part of setup
					expectedLayerNum--

					return nil
				}

				imageID, envVars, _, err := fetcher.Fetch(
					logger,
					parseURL("scheme://host/some-repo"),
					"some-tag",
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(envVars).To(Equal(process.Env{"env2": "env2Value"}))

				Expect(imageID).To(Equal("id-1"))
			})
		})

		Context("when fetching repository data fails", func() {
			BeforeEach(func() {
				server.SetHandler(1, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { // first request after ping
					w.WriteHeader(500)
				}))
			})

			It("returns an error", func() {
				_, _, _, err := fetcher.Fetch(
					logger,
					parseURL("scheme://host/some-repo"),
					"some-tag",
				)
				Expect(err.Error()).To(ContainSubstring("repository_fetcher: GetRepositoryData: could not fetch image some-repo from registry some-repo:"))
			})
		})

		Context("when fetching the remote tags fails", func() {
			BeforeEach(func() {
				endpoint1.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(500)
				}))

				endpoint2.AppendHandlers(
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

				setupSuccessfulFetch(endpoint1)
			})

			It("tries the next endpoint", func() {
				_, _, _, err := fetcher.Fetch(
					logger,
					parseURL("scheme://host/some-repo"),
					"some-tag",
				)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("on all endpoints", func() {
				BeforeEach(func() {
					endpoint2.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(500)
					}))
				})

				It("returns an error", func() {
					_, _, _, err := fetcher.Fetch(
						logger,
						parseURL("scheme://host/some-repo"),
						"some-tag",
					)
					Expect(err.Error()).To(ContainSubstring("repository_fetcher: GetRemoteTags: could not fetch image some-repo from registry some-repo:"))
				})
			})
		})
	})
})

func parseURL(str string) *url.URL {
	parsedURL, err := url.Parse(str)
	Expect(err).ToNot(HaveOccurred())

	return parsedURL
}
