package repository_fetcher_test

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/docker/docker/archive"
	"github.com/docker/docker/image"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/fake_graph"
	. "github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/repository_fetcher"
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

	BeforeEach(func() {
		graph = fake_graph.New()

		server = ghttp.NewServer()

		endpoint1 = ghttp.NewServer()
		endpoint2 = ghttp.NewServer()

		registry, err := registry.NewSession(nil, nil, server.URL()+"/v1/", true)
		Ω(err).ShouldNot(HaveOccurred())

		fetcher = New(registry, graph)

		logger = lagertest.NewTestLogger("test")
	})

	setupSuccessfulFetch := func(endpoint *ghttp.Server) {
		endpoint.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/images/layer-3/json"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Add("X-Docker-Size", "123")
					w.Write([]byte(`{"id":"layer-3","parent":"parent-3","Config":{"env": ["env2=env2Value"]}}`))
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
					w.Write([]byte(`{"id":"layer-2","parent":"parent-2","Config":{"env": ["env1=env1Value", "env2=env2BadValue"]}}`))
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

		Context("when none of the layers already exist", func() {
			BeforeEach(func() {
				setupSuccessfulFetch(endpoint1)
			})

			It("downloads all layers of the given tag of a repository and returns its image id", func() {
				expectedLayerNum := 3

				graph.WhenRegistering = func(image *image.Image, imageJSON []byte, layer archive.ArchiveReader) error {
					if expectedLayerNum == 3 {
						Ω(string(imageJSON)).Should(Equal(fmt.Sprintf(
							`{"id":"layer-%d","parent":"parent-%d","Config":{"env": ["env2=env2Value"]}}`,
							expectedLayerNum,
							expectedLayerNum,
						)))
					} else if expectedLayerNum == 2 {
						Ω(string(imageJSON)).Should(Equal(fmt.Sprintf(
							`{"id":"layer-%d","parent":"parent-%d","Config":{"env": ["env1=env1Value", "env2=env2BadValue"]}}`,
							expectedLayerNum,
							expectedLayerNum,
						)))
					} else {
						Ω(string(imageJSON)).Should(Equal(fmt.Sprintf(
							`{"id":"layer-%d","parent":"parent-%d"}`,
							expectedLayerNum,
							expectedLayerNum,
						)))
					}

					Ω(image.ID).Should(Equal(fmt.Sprintf("layer-%d", expectedLayerNum)))
					Ω(image.Parent).Should(Equal(fmt.Sprintf("parent-%d", expectedLayerNum)))

					layerData, err := ioutil.ReadAll(layer)
					Ω(err).ShouldNot(HaveOccurred())
					Ω(string(layerData)).Should(Equal(fmt.Sprintf("layer-%d-data", expectedLayerNum)))

					expectedLayerNum--

					return nil
				}

				imageID, envvars, err := fetcher.Fetch(logger, "some-repo", "some-tag")

				Ω(err).ShouldNot(HaveOccurred())
				Ω(envvars).Should(ContainElement("env2=env2Value"))
				Ω(envvars).Should(ContainElement("env1=env1Value"))
				Ω(imageID).Should(Equal("id-1"))
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
					imageID, _, err := fetcher.Fetch(logger, "some-repo", "some-tag")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(imageID).Should(Equal("id-1"))
				})

				Context("and the rest also fail", func() {
					BeforeEach(func() {
						endpoint2.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.WriteHeader(500)
						}))
					})

					It("returns an error", func() {
						_, _, err := fetcher.Fetch(logger, "some-repo", "some-tag")
						Ω(err).Should(HaveOccurred())
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

				graph.WhenRegistering = func(image *image.Image, imageJSON []byte, layer archive.ArchiveReader) error {
					Ω(string(imageJSON)).Should(Equal(fmt.Sprintf(
						`{"id":"layer-%d","parent":"parent-%d"}`,
						expectedLayerNum,
						expectedLayerNum,
					)))

					Ω(image.ID).Should(Equal(fmt.Sprintf("layer-%d", expectedLayerNum)))
					Ω(image.Parent).Should(Equal(fmt.Sprintf("parent-%d", expectedLayerNum)))

					layerData, err := ioutil.ReadAll(layer)
					Ω(err).ShouldNot(HaveOccurred())
					Ω(string(layerData)).Should(Equal(fmt.Sprintf("layer-%d-data", expectedLayerNum)))

					expectedLayerNum--

					// skip 2 as it already exists as part of setup
					expectedLayerNum--

					return nil
				}

				imageID, envVars, err := fetcher.Fetch(logger, "some-repo", "some-tag")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(envVars).Should(ContainElement("env2=env2Value"))

				Ω(imageID).Should(Equal("id-1"))
			})
		})

		Context("when fetching repository data fails", func() {
			BeforeEach(func() {
				server.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(500)
				}))
			})

			It("returns an error", func() {
				_, _, err := fetcher.Fetch(logger, "some-repo", "some-tag")
				Ω(err).Should(HaveOccurred())
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
				_, _, err := fetcher.Fetch(logger, "some-repo", "some-tag")
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("on all endpoints", func() {
				BeforeEach(func() {
					endpoint2.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(500)
					}))
				})

				It("returns an error", func() {
					_, _, err := fetcher.Fetch(logger, "some-repo", "some-tag")
					Ω(err).Should(HaveOccurred())
				})
			})
		})
	})
})
