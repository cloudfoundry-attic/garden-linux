package repository_fetcher_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_cake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_retainer"
	. "github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_lock"
	"github.com/docker/distribution/digest"
	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/transport"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager/lagertest"

	"math"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("RemoteV2", func() {
	var (
		fetcher      *RemoteV2Fetcher
		server       *ghttp.Server
		cake         *fake_cake.FakeCake
		lock         *fake_lock.FakeLock
		logger       *lagertest.TestLogger
		fetchRequest *FetchRequest
		retainer     *fake_retainer.FakeRetainer

		registryAddr string
	)

	BeforeEach(func() {
		cake = new(fake_cake.FakeCake)
		lock = new(fake_lock.FakeLock)

		logger = lagertest.NewTestLogger("test")

		server = ghttp.NewServer()
		server.RouteToHandler(
			"GET", "/v2/",
			http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
				w.Write([]byte(`{}`))
			}),
		)
		server.RouteToHandler(
			"GET", "/v1/_ping",
			http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(404)
			}),
		)

		registryAddr = server.HTTPTestServer.Listener.Addr().String()
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

		fetchRequest = &FetchRequest{
			Session:    session,
			Endpoint:   endpoint,
			Logger:     logger,
			Path:       "some-repo",
			RemotePath: "some-repo",
			Tag:        "some-tag",
			MaxSize:    math.MaxInt64,
		}

		retainer = new(fake_retainer.FakeRetainer)
		fetcher = &RemoteV2Fetcher{
			Cake:      cake,
			Retainer:  retainer,
			GraphLock: lock,
		}

		cake.GetReturns(nil, errors.New("no image"))
	})

	It("retains the layers before getting them, to ensure they are not deleted after we decide to use cache", func() {
		setupSuccessfulV2Fetch(server, false)

		retained := make(map[layercake.ID]bool)
		cake.GetStub = func(id layercake.ID) (*image.Image, error) {
			Expect(retained).To(HaveKey(id))
			return nil, errors.New("no layer")
		}

		retainer.RetainStub = func(id layercake.ID) {
			retained[id] = true
		}

		fetcher.Fetch(fetchRequest)
	})

	It("releases all the layers after fetching", func() {
		setupSuccessfulV2Fetch(server, false)

		released := make(map[layercake.ID]bool)
		retainer.ReleaseStub = func(id layercake.ID) {
			released[id] = true
		}

		cake.GetStub = func(id layercake.ID) (*image.Image, error) {
			Expect(released).To(BeEmpty())
			return nil, errors.New("no layer")
		}

		fetcher.Fetch(fetchRequest)

		Expect(released).To(HaveKey(layercake.DockerImageID("banana-pie-1")))
		Expect(released).To(HaveKey(layercake.DockerImageID("banana-pie-2")))
	})

	Context("when none of the layers already exist", func() {
		BeforeEach(func() {
			setupSuccessfulV2Fetch(server, false)
		})

		It("downloads all layers of the given tag of a repository and returns its image id", func() {
			layers := 0

			cake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
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

			fetchResponse, err := fetcher.Fetch(fetchRequest)

			Expect(err).ToNot(HaveOccurred())
			Expect(fetchResponse.Env).To(BeEmpty())
			Expect(fetchResponse.Volumes).To(BeEmpty())
			Expect(fetchResponse.ImageID).To(Equal("banana-pie-2"))

			Expect(server.ReceivedRequests()).To(HaveLen(4))
			Expect(layers).To(Equal(2))
		})

		It("returns a quota exceeded error if the layers exceed the quota", func() {
			fetchRequest.MaxSize = 65
			called := 0
			cake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
				Expect(layer).To(BeAssignableToTypeOf(&QuotaedReader{}))
				image.Size = 33

				if called == 0 {
					Expect(layer.(*QuotaedReader).N).To(Equal(int64(65)))
				} else {
					Expect(layer.(*QuotaedReader).N).To(Equal(int64(65 - 33)))
				}

				called++
				return nil
			}

			_, err := fetcher.Fetch(fetchRequest)
			Expect(err).To(MatchError("quota exceeded"))
		})
	})

	Context("when a layer already exists", func() {
		BeforeEach(func() {
			cake.GetStub = func(id layercake.ID) (*image.Image, error) {
				if id.GraphID() != "banana-pie-1" {
					return nil, errors.New("no layer")
				}

				return &image.Image{ID: "banana-pie-1"}, nil
			}
			setupSuccessfulV2Fetch(server, true)
		})

		Context("and it is larger than the quota", func() {
			BeforeEach(func() {
				fetchRequest.MaxSize = 43
			})

			It("returns a quota exceeded error", func() {
				cake.GetReturns(&image.Image{
					ID:   "banana-pie-1",
					Size: 44,
				}, nil)
				setupSuccessfulV2Fetch(server, true)

				_, err := fetcher.Fetch(fetchRequest)
				Expect(err).To(MatchError("quota exceeded"))
			})
		})

		It("is not added to the graph", func() {
			layers := 0

			cake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
				Expect(image.ID).To(Equal("banana-pie-2"))
				Expect(image.Parent).To(Equal("banana-pie-1"))

				layerData, err := ioutil.ReadAll(layer)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(layerData)).To(Equal("banana-2-flan"))

				layers++

				return nil
			}

			_, err := fetcher.Fetch(fetchRequest)

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
				_, err := fetcher.Fetch(fetchRequest)
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
				_, err := fetcher.Fetch(fetchRequest)
				Expect(err).To(MatchError(ContainSubstring("repository_fetcher: UnmarshalManifest: could not fetch image some-repo from registry %s:", registryAddr)))
			})
		})
	})

	Context("when fetching a layer fails", func() {
		Context("when the image manifest contains an invalid layer digest", func() {
			BeforeEach(func() {
				setupSuccessfulV2Fetch(server, false)
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
				_, err := fetcher.Fetch(fetchRequest)
				Expect(err).To(MatchError(ContainSubstring("invalid checksum digest format")))
			})
		})

		Context("when the image JSON is invalid", func() {
			BeforeEach(func() {
				setupSuccessfulV2Fetch(server, false)
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
				_, err := fetcher.Fetch(fetchRequest)
				Expect(err).To(MatchError(ContainSubstring("repository_fetcher: NewImgJSON: could not fetch image some-repo from registry %s:", registryAddr)))
			})
		})

		Context("when downloading the blob fails", func() {
			BeforeEach(func() {
				setupSuccessfulV2Fetch(server, false)

				server.SetHandler(1, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(500)
				}))
			})

			It("returns an error", func() {
				_, err := fetcher.Fetch(fetchRequest)
				Expect(err).To(MatchError(ContainSubstring("repository_fetcher: GetV2ImageBlobReader: could not fetch image some-repo from registry %s:", registryAddr)))
			})
		})

		Context("when registering the layer with the graph fails", func() {
			BeforeEach(func() {
				setupSuccessfulV2Fetch(server, false)
				cake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
					return errors.New("oh no!")
				}
			})

			It("returns error", func() {
				_, err := fetcher.Fetch(fetchRequest)
				Expect(err).To(MatchError(ContainSubstring("oh no!")))
			})
		})
	})
})

func setupSuccessfulV2Fetch(server *ghttp.Server, layer1Cached bool) {
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
