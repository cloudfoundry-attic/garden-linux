package repository_fetcher_test

import (
	"errors"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/container_pool/fake_graph"
	"github.com/cloudfoundry-incubator/garden-linux/old/repository_fetcher"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("MD5ID", func() {
	It("returns the MD5 sum of the path", func() {
		ider := repository_fetcher.MD5ID{}
		Expect(ider.ID("something")).To(Equal("437b930db84b8079c2dd804a71936b5f"))
	})
})

var _ = Describe("Local", func() {
	var fetcher *repository_fetcher.Local
	var fakeGraph *fake_graph.FakeGraph
	var defaultRootFSPath string
	var logger lager.Logger

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("local")
		fakeGraph = fake_graph.New()
		defaultRootFSPath = ""
	})

	JustBeforeEach(func() {
		fetcher = &repository_fetcher.Local{
			Graph:             fakeGraph,
			IDer:              UnderscoreIDer{},
			DefaultRootFSPath: defaultRootFSPath,
		}
	})

	Context("when the image already exists in the graph", func() {
		It("returns the image id", func() {
			fakeGraph.SetExists("foo_bar_baz", []byte("{}"))

			id, _, _, err := fetcher.Fetch(logger, &url.URL{Path: "foo/bar/baz"}, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal("foo_bar_baz"))
		})

		Context("when the path is empty", func() {
			Context("and a default was specified", func() {
				BeforeEach(func() {
					defaultRootFSPath = "the/default/path"
				})

				It("should use the default", func() {
					fakeGraph.SetExists("the_default_path", []byte("{}"))

					id, _, _, err := fetcher.Fetch(logger, &url.URL{Path: ""}, "")
					Expect(err).NotTo(HaveOccurred())
					Expect(id).To(Equal("the_default_path"))
				})
			})

			Context("and a default was not specified", func() {
				It("should throw an appropriate error", func() {
					_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: ""}, "")
					Expect(err).To(MatchError("RootFSPath: is a required parameter, since no default rootfs was provided to the server. To provide a default rootfs, use the --rootfs flag on startup."))
				})
			})
		})
	})

	Context("when the image does not already exist", func() {
		It("registers the image in the graph", func() {
			var registeredImage *image.Image
			fakeGraph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
				registeredImage = image
				return nil
			}

			_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: "foo/bar/baz"}, "")
			Expect(err).NotTo(HaveOccurred())

			Expect(registeredImage).NotTo(BeNil())
			Expect(registeredImage.ID).To(Equal("foo_bar_baz"))
		})

		It("registers the image with the correct layer data", func() {
			var registeredLayer archive.ArchiveReader
			fakeGraph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
				registeredLayer = layer
				return nil
			}

			tmp, err := ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			Expect(os.MkdirAll(path.Join(tmp, "a", "test"), 0700)).To(Succeed())
			Expect(ioutil.WriteFile(path.Join(tmp, "a", "test", "file"), []byte(""), 0700)).To(Succeed())

			_, _, _, err = fetcher.Fetch(logger, &url.URL{Path: tmp}, "")
			Expect(err).NotTo(HaveOccurred())

			tmp, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			Expect(archive.Untar(registeredLayer, tmp, nil)).To(Succeed())
			Expect(path.Join(tmp, "a", "test", "file")).To(BeAnExistingFile())
		})

		It("returns a wrapped error if registering fails", func() {
			fakeGraph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
				return errors.New("sold out")
			}

			_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: "foo/bar/baz"}, "")
			Expect(err).To(MatchError("fetch local rootfs: sold out"))
		})

		It("returns the image id", func() {
			id, _, _, err := fetcher.Fetch(logger, &url.URL{Path: "foo/bar/baz"}, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal("foo_bar_baz"))
		})
	})
})

type UnderscoreIDer struct{}

func (UnderscoreIDer) ID(path string) string {
	return strings.Replace(path, "/", "_", -1)
}
