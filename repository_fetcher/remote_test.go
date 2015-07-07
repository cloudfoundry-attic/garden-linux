package repository_fetcher_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/transport"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden-linux/process"
	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/resource_pool/fake_graph"
	"github.com/docker/distribution/digest"
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

		server.RouteToHandler(
			"GET", "/v1/_ping", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-Docker-Registry-Version", "v1")
				w.Header().Add("X-Docker-Registry-Standalone", "true")
				w.Write([]byte(`{"standalone": true, "version": "v1"}`))
			}),
		)
		server.RouteToHandler(
			"GET", "/v2/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(404)
			}),
		)
	})

	JustBeforeEach(func() {
		endpoint, err := registry.NewEndpoint(&registry.IndexInfo{
			Name:   server.HTTPTestServer.Listener.Addr().String(),
			Secure: false,
		}, nil)
		Expect(err).ToNot(HaveOccurred())

		tr := transport.NewTransport(
			registry.NewTransport(registry.ReceiveTimeout, endpoint.IsSecure),
		)

		r, err := registry.NewSession(registry.HTTPClient(tr), &cliconfig.AuthConfig{}, endpoint)
		Expect(err).ToNot(HaveOccurred())

		fakeRegistryProvider = new(fakes.FakeRegistryProvider)
		fakeRegistryProvider.ApplyDefaultHostnameReturns("some-repo")
		fakeRegistryProvider.ProvideRegistryReturns(r, endpoint, nil)
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

	setupSuccessfulV2Fetch := func(layer1Cached bool) {
		layer1Data := "banana-1-flan"
		layer1Dgst, _ := digest.FromBytes([]byte(layer1Data))

		layer2Data := "banana-2-flan"
		layer2Dgst, _ := digest.FromBytes([]byte(layer2Data))

		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v2/some-repo/manifests/some-tag"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Write([]byte(fmt.Sprintf(`
					{
					   "name":"some-repo",
					   "tag":"some-tag",
					   "fsLayers":[
						  {
							 "blobSum":"%s"
						  },
						  {
							 "blobSum":"%s"
						  }
					   ],
					   "history":[
						  {
							 "v1Compatibility": "{\"id\":\"banana-pie-2\", \"parent\":\"banana-pie-1\"}"
						  },
						  {
							 "v1Compatibility": "{\"id\":\"banana-pie-1\"}"
						  }
					   ]
					}
					`, layer2Dgst.String(), layer1Dgst.String())))
				}),
			),
		)

		if !layer1Cached {
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", fmt.Sprintf("/v2/some-repo/blobs/%s", layer1Dgst)),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Write([]byte(layer1Data))
					}),
				),
			)
		}

		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", fmt.Sprintf("/v2/some-repo/blobs/%s", layer2Dgst)),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Write([]byte(layer2Data))
				}),
			),
		)
	}

	Describe("Fetch", func() {
		Context("when the path is empty", func() {
			It("returns an error", func() {
				_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: ""}, "some-tag")
				Expect(err).To(Equal(ErrInvalidDockerURL))
			})
		})

		Describe("connecting to the correct registry", func() {
			JustBeforeEach(func() {
				server.AllowUnhandledRequests = true
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
					fakeRegistryProvider.ProvideRegistryReturns(nil, nil, errors.New("an error"))

					_, _, _, err := fetcher.Fetch(logger, parseURL("some-scheme://some-registry:4444/some-repo"), "some-tag")
					Expect(err).To(MatchError("repository_fetcher: ProvideRegistry: could not fetch image some-repo from registry some-registry:4444: an error"))
				})
			})
		})

		Describe("v1 registries", func() {
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
							Expect(err).To(MatchError(ContainSubstring("repository_fetcher: fetchFromEndPoint: could not fetch image some-repo from registry some-repo: all endpoints failed:")))
						})
					})
				})
			})

			Context("when a layer already exists", func() {
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

				It("is not added to the graph", func() {
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
					server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(500)
					}))
				})

				It("returns an error", func() {
					_, _, _, err := fetcher.Fetch(
						logger,
						parseURL("scheme://host/some-repo"),
						"some-tag",
					)
					Expect(err).To(MatchError(ContainSubstring("repository_fetcher: GetRepositoryData: could not fetch image some-repo from registry some-repo:")))
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
						Expect(err).To(MatchError(ContainSubstring("repository_fetcher: GetRemoteTags: could not fetch image some-repo from registry some-repo:")))
					})
				})
			})
		})

		Describe("v2 registries", func() {
			BeforeEach(func() {
				server.RouteToHandler(
					"GET", "/v1/_ping",
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(404)
					}),
				)
				server.RouteToHandler(
					"GET", "/v2/",
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
						w.Write([]byte(`{}`))
					}),
				)
			})

			Context("when none of the layers already exist", func() {

				JustBeforeEach(func() {
					fakeRegistryProvider.ApplyDefaultHostnameReturns("some-registry:4444")
					setupSuccessfulV2Fetch(false)
				})

				It("downloads all layers of the given tag of a repository and returns its image id", func() {
					layers := 0

					graph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
						Expect(image.ID).To(Equal(fmt.Sprintf("banana-pie-%d", layers+1)))
						parent := ""
						if layers > 0 {
							parent = fmt.Sprintf("banana-pie-%d", layers)
						}
						Expect(image.Parent).To(Equal(parent))

						layerData, err := ioutil.ReadAll(layer)
						Expect(err).ToNot(HaveOccurred())
						Expect(string(layerData)).To(Equal(fmt.Sprintf("banana-%d-flan", layers+1)))

						layers++

						return nil
					}

					imageID, envvars, volumes, err := fetcher.Fetch(
						logger,
						parseURL("scheme://host/some-repo"),
						"some-tag",
					)

					Expect(err).ToNot(HaveOccurred())
					Expect(envvars).To(Equal(process.Env{}))
					Expect(volumes).To(ConsistOf([]string{}))
					Expect(imageID).To(Equal("banana-pie-2"))

					Expect(server.ReceivedRequests()).To(HaveLen(4))
					Expect(layers).To(Equal(2))
				})
			})

			Context("when a layer already exists", func() {
				BeforeEach(func() {
					graph.SetExists("banana-pie-1", []byte(`{"id": "banana-pie-1"}`))
				})

				JustBeforeEach(func() {
					fakeRegistryProvider.ApplyDefaultHostnameReturns("some-registry:4444")
					setupSuccessfulV2Fetch(true)
				})

				It("is not added to the graph", func() {
					layers := 0

					graph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
						Expect(image.ID).To(Equal("banana-pie-2"))
						Expect(image.Parent).To(Equal("banana-pie-1"))

						layerData, err := ioutil.ReadAll(layer)
						Expect(err).ToNot(HaveOccurred())
						Expect(string(layerData)).To(Equal("banana-2-flan"))

						layers++

						return nil
					}

					_, _, _, err := fetcher.Fetch(
						logger,
						parseURL("scheme://host/some-repo"),
						"some-tag",
					)

					Expect(err).ToNot(HaveOccurred())
					Expect(server.ReceivedRequests()).To(HaveLen(3))
					Expect(layers).To(Equal(1))
				})
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
						_, _, _, err := fetcher.Fetch(
							logger,
							parseURL("scheme://host/some-repo"),
							"some-tag",
						)
						Expect(err).To(MatchError(ContainSubstring("repository_fetcher: GetV2ImageManifest: could not fetch image some-repo from registry some-repo:")))
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
						_, _, _, err := fetcher.Fetch(
							logger,
							parseURL("scheme://host/some-repo"),
							"some-tag",
						)
						Expect(err).To(MatchError(ContainSubstring("repository_fetcher: UnmarshalManifest: could not fetch image some-repo from registry some-repo:")))
					})
				})
			})

			Context("when fetching a layer fails", func() {
				Context("when the image manifest contains an invalid layer digest", func() {
					BeforeEach(func() {
						setupSuccessfulV2Fetch(false)
						server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Write([]byte(`
					{
					   "name":"some-repo",
					   "tag":"some-tag",
					   "fsLayers":[
						  {
							 "blobSum":"barry"
						  }
					   ],
					   "history":[
						  {
							 "v1Compatibility": "{\"id\":\"banana-pie-2\"}"
						  }
					   ]
					}`))
						}))
					})

					It("returns an error", func() {
						_, _, _, err := fetcher.Fetch(
							logger,
							parseURL("scheme://host/some-repo"),
							"some-tag",
						)
						Expect(err).To(MatchError(ContainSubstring("invalid checksum digest format")))
					})
				})

				Context("when the image JSON is invalid", func() {
					BeforeEach(func() {
						setupSuccessfulV2Fetch(false)
						server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Write([]byte(`
					{
					   "name":"some-repo",
					   "tag":"some-tag",
					   "fsLayers":[
						  {
							 "blobSum":"sha256:7b3bc336724d50e1ad47bde1458ab57d170a453b0ed734087734a8eaf79c1274"
						  }
					   ],
					   "history":[
						  {
							 "v1Compatibility": "{ba}"
						  }
					   ]
					}`))
						}))
					})

					It("returns an error", func() {
						_, _, _, err := fetcher.Fetch(
							logger,
							parseURL("scheme://host/some-repo"),
							"some-tag",
						)
						Expect(err).To(MatchError(ContainSubstring("repository_fetcher: NewImgJSON: could not fetch image some-repo from registry some-repo:")))
					})
				})

				Context("when downloading the blob fails", func() {
					BeforeEach(func() {
						setupSuccessfulV2Fetch(false)

						server.SetHandler(1, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.WriteHeader(500)
						}))
					})

					It("returns an error", func() {
						_, _, _, err := fetcher.Fetch(
							logger,
							parseURL("scheme://host/some-repo"),
							"some-tag",
						)
						Expect(err).To(MatchError(ContainSubstring("repository_fetcher: GetV2ImageBlobReader: could not fetch image some-repo from registry some-repo:")))
					})
				})

				Context("when registering the layer with the graph fails", func() {
					BeforeEach(func() {
						setupSuccessfulV2Fetch(false)
						graph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
							return errors.New("oh no!")
						}
					})

					It("returns error", func() {
						_, _, _, err := fetcher.Fetch(
							logger,
							parseURL("scheme://host/some-repo"),
							"some-tag",
						)
						Expect(err).To(MatchError(ContainSubstring("oh no!")))
					})
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
