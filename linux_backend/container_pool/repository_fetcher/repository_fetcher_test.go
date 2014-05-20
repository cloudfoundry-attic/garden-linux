package repository_fetcher_test

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/dotcloud/docker/archive"
	"github.com/dotcloud/docker/image"
	"github.com/dotcloud/docker/registry"

	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/fake_graph"
	. "github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/repository_fetcher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("RepositoryFetcher", func() {
	var graph *fake_graph.FakeGraph
	var fetcher RepositoryFetcher

	var server *ghttp.Server
	var endpoint1 *ghttp.Server
	var endpoint2 *ghttp.Server

	BeforeEach(func() {
		graph = fake_graph.New()

		server = ghttp.NewServer()

		endpoint1 = ghttp.NewServer()
		endpoint2 = ghttp.NewServer()

		registry, err := registry.NewRegistry(nil, nil, server.URL()+"/v1/")
		Ω(err).ShouldNot(HaveOccurred())

		fetcher = New(registry, graph)
	})

	setupSuccessfulFetch := func(endpoint *ghttp.Server) {
		endpoint.AppendHandlers(
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
				ghttp.VerifyRequest("GET", "/v1/images/layer-2/json"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Add("X-Docker-Size", "456")
					w.Write([]byte(`{"id":"layer-2","parent":"parent-2"}`))
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

				graph.WhenRegistering = func(imageJSON []byte, layer archive.ArchiveReader, image *image.Image) error {
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

					return nil
				}

				imageID, err := fetcher.Fetch("some-repo", "some-tag")
				Ω(err).ShouldNot(HaveOccurred())

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
					imageID, err := fetcher.Fetch("some-repo", "some-tag")
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
						_, err := fetcher.Fetch("some-repo", "some-tag")
						Ω(err).Should(HaveOccurred())
					})
				})
			})
		})

		Context("when an image already exists in the graph", func() {
			BeforeEach(func() {
				graph.SetExists("layer-2", true)

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

				graph.WhenRegistering = func(imageJSON []byte, layer archive.ArchiveReader, image *image.Image) error {
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

				imageID, err := fetcher.Fetch("some-repo", "some-tag")
				Ω(err).ShouldNot(HaveOccurred())

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
				_, err := fetcher.Fetch("some-repo", "some-tag")
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
				_, err := fetcher.Fetch("some-repo", "some-tag")
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("on all endpoints", func() {
				BeforeEach(func() {
					endpoint2.SetHandler(0, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(500)
					}))
				})

				It("returns an error", func() {
					_, err := fetcher.Fetch("some-repo", "some-tag")
					Ω(err).Should(HaveOccurred())
				})
			})
		})
	})
})
