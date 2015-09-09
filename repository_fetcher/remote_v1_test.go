package repository_fetcher_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_cake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_retainer"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_lock"
	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/transport"
	"github.com/docker/docker/registry"
	"github.com/docker/docker/runconfig"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("RemoteV1", func() {
	var (
		fetcher         *RemoteV1Fetcher
		server          *ghttp.Server
		endpoint1Server *ghttp.Server
		endpoint2Server *ghttp.Server
		cake            *fake_cake.FakeCake
		lock            *fake_lock.FakeLock
		logger          *lagertest.TestLogger
		fetchRequest    *FetchRequest
		retainer        *fake_retainer.FakeRetainer

		registryAddr string
	)

	BeforeEach(func() {
		cake = new(fake_cake.FakeCake)
		lock = new(fake_lock.FakeLock)
		logger = lagertest.NewTestLogger("test")
		server, endpoint1Server, endpoint2Server, registryAddr, fetchRequest = createFakeHTTPV1RegistryServer(logger)

		retainer = new(fake_retainer.FakeRetainer)
		fetcher = &RemoteV1Fetcher{
			Cake:             cake,
			MetadataProvider: &ImageV1MetadataProvider{},
			Retainer:         retainer,
			GraphLock:        lock,
		}

		cake.GetReturns(&image.Image{}, nil)
	})

	Describe("Fetch", func() {
		It("retains all the layers before starting", func() {
			setupSuccessfulFetch(endpoint1Server)

			retained := make(map[layercake.ID]bool)
			cake.GetStub = func(id layercake.ID) (*image.Image, error) {
				Expect(retained).To(HaveKey(layercake.DockerImageID("layer-1")))
				Expect(retained).To(HaveKey(layercake.DockerImageID("layer-2")))
				Expect(retained).To(HaveKey(layercake.DockerImageID("layer-3")))
				return nil, errors.New("no layer")
			}

			retainer.RetainStub = func(id layercake.ID) {
				retained[id] = true
			}

			fetcher.Fetch(fetchRequest)
		})

		It("releases all the layers after fetching", func() {
			setupSuccessfulFetch(endpoint1Server)

			released := make(map[layercake.ID]bool)
			retainer.ReleaseStub = func(id layercake.ID) {
				released[id] = true
			}

			cake.GetStub = func(id layercake.ID) (*image.Image, error) {
				Expect(released).To(BeEmpty())
				return nil, errors.New("no layer")
			}

			fetcher.Fetch(fetchRequest)

			Expect(released).To(HaveKey(layercake.DockerImageID("layer-1")))
			Expect(released).To(HaveKey(layercake.DockerImageID("layer-2")))
			Expect(released).To(HaveKey(layercake.DockerImageID("layer-3")))
		})

		Context("when none of the layers already exist", func() {
			BeforeEach(func() {
				setupSuccessfulFetch(endpoint1Server)
				cake.GetReturns(nil, errors.New("no layer"))
			})

			It("downloads all layers of the given tag of a repository and returns its image id", func() {
				expectedLayerNum := 3

				cake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
					Expect(image.ID).To(Equal(fmt.Sprintf("layer-%d", expectedLayerNum)))
					Expect(image.Parent).To(Equal(fmt.Sprintf("parent-%d", expectedLayerNum)))

					layerData, err := ioutil.ReadAll(layer)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(layerData)).To(Equal(fmt.Sprintf("layer-%d-data", expectedLayerNum)))

					expectedLayerNum--

					return nil
				}

				fetchResponse, err := fetcher.Fetch(fetchRequest)

				Expect(err).ToNot(HaveOccurred())
				Expect(fetchResponse.Env).To(Equal(process.Env{"env1": "env1Value", "env2": "env2NewValue"}))
				Expect(fetchResponse.Volumes).To(ConsistOf([]string{"/tmp", "/another"}))
				Expect(fetchResponse.ImageID).To(Equal("id-1"))
			})

			Context("when the layers exceed the quota", func() {
				BeforeEach(func() {
					fetchRequest.MaxSize = 87
				})

				It("should return a quota exceeded error", func() {
					called := 0
					cake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
						Expect(layer).To(BeAssignableToTypeOf(&QuotaedReader{}))
						image.Size = 44

						if called == 0 {
							Expect(layer.(*QuotaedReader).N).To(Equal(int64(87)))
						} else {
							Expect(layer.(*QuotaedReader).N).To(Equal(int64(87 - 44)))
						}

						called++

						return nil
					}

					_, err := fetcher.Fetch(fetchRequest)
					Expect(err).To(MatchError("quota exceeded"))
				})

				It("should not download further layers", func() {
					registered := 0
					cake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
						image.Size = 44
						registered++

						return nil
					}

					fetcher.Fetch(fetchRequest)
					Expect(registered).To(Equal(2))
				})
			})

			Context("when the first endpoint fails", func() {
				BeforeEach(func() {
					endpoint1Server.SetHandler(1, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(500)
					}))

					endpoint2Server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/v1/images/id-1/ancestry"),
							http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
								w.Write([]byte(`["layer-1", "layer-2", "layer-3"]`))
							}),
						),
					)

					setupSuccessfulFetch(endpoint2Server)
				})

				It("retries with the next endpoint", func() {
					fetchResponse, err := fetcher.Fetch(fetchRequest)

					Expect(err).ToNot(HaveOccurred())
					Expect(fetchResponse.ImageID).To(Equal("id-1"))
				})

				Context("and the rest also fail", func() {
					BeforeEach(func() {
						endpoint2Server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.WriteHeader(500)
						}))
					})

					It("returns an error", func() {
						_, err := fetcher.Fetch(fetchRequest)
						Expect(err).To(MatchError(ContainSubstring("repository_fetcher: fetchFromEndPoint: could not fetch image some-repo from registry %s: all endpoints failed:", registryAddr)))
					})
				})
			})
		})

		Context("when a layer already exists", func() {
			BeforeEach(func() {
				cake.GetStub = func(id layercake.ID) (*image.Image, error) {
					if id.GraphID() == "layer-2" {
						return &image.Image{
							Parent: "parent-2",
							Config: &runconfig.Config{
								Env: []string{"env2=env2Value"},
							},
						}, nil
					}

					return &image.Image{}, nil
				}

				endpoint1Server.AppendHandlers(
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

			Context("and it is bigger than the quota", func() {
				BeforeEach(func() {
					fetchRequest.MaxSize = 12344
				})

				It("should return a quota exceeded error", func() {
					cake.GetReturns(&image.Image{Size: 12345}, nil)

					_, err := fetcher.Fetch(fetchRequest)
					Expect(err).To(MatchError("quota exceeded"))
				})
			})

			It("is not added to the graph", func() {
				expectedLayerNum := 3

				cake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
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

				fetchResponse, err := fetcher.Fetch(fetchRequest)

				Expect(err).ToNot(HaveOccurred())
				Expect(fetchResponse.Env).To(Equal(process.Env{"env2": "env2Value"}))
				Expect(fetchResponse.ImageID).To(Equal("id-1"))
			})
		})

		Context("when fetching repository data fails", func() {
			BeforeEach(func() {
				server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(500)
				}))
			})

			It("returns an error", func() {
				_, err := fetcher.Fetch(fetchRequest)
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

			It("tries the next endpoint", func() {
				_, err := fetcher.Fetch(fetchRequest)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("on all endpoints", func() {
				BeforeEach(func() {
					endpoint2Server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(500)
					}))
				})

				It("returns an error", func() {
					_, err := fetcher.Fetch(fetchRequest)
					Expect(err).To(MatchError(ContainSubstring("repository_fetcher: GetRemoteTags: could not fetch image some-repo from registry %s:", registryAddr)))
				})
			})
		})
	})
})

func setupSuccessfulFetch(server *ghttp.Server) {
	server.AppendHandlers(
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

func createFakeHTTPV1RegistryServer(logger lager.Logger) (*ghttp.Server, *ghttp.Server, *ghttp.Server, string, *FetchRequest) {
	server := ghttp.NewServer()
	endpoint1Server := ghttp.NewServer()
	endpoint2Server := ghttp.NewServer()

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
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/v1/repositories/some-repo/images"),
			http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-Docker-Token", "token-1,token-2")
				w.Header().Add("X-Docker-Endpoints", endpoint1Server.HTTPTestServer.Listener.Addr().String())
				w.Header().Add("X-Docker-Endpoints", endpoint2Server.HTTPTestServer.Listener.Addr().String())
				w.Write([]byte(`[
							{"id": "id-1", "checksum": "sha-1"},
							{"id": "id-2", "checksum": "sha-2"}
						]`))
			}),
		),
	)

	endpoint1Server.AppendHandlers(
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

	registryAddr := server.HTTPTestServer.Listener.Addr().String()
	endpoint, err := registry.NewEndpoint(&registry.IndexInfo{
		Name:   registryAddr,
		Secure: false,
	}, nil)
	Expect(err).ToNot(HaveOccurred())

	tr := transport.NewTransport(
		registry.NewTransport(registry.ReceiveTimeout, endpoint.IsSecure),
	)

	session, err := registry.NewSession(registry.HTTPClient(tr), &cliconfig.AuthConfig{}, endpoint)
	Expect(err).ToNot(HaveOccurred())

	return server, endpoint1Server, endpoint2Server, registryAddr, &FetchRequest{
		Session:  session,
		Endpoint: endpoint,
		Logger:   logger,
		Path:     "some-repo",
		Tag:      "some-tag",
		MaxSize:  99999,
	}
}
