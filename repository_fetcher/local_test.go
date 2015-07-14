package repository_fetcher_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"strconv"

	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/resource_pool/fake_graph"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("SHA256", func() {
	var path string
	var accessTime time.Time
	var ider repository_fetcher.SHA256
	const pathHash = "ebdc0142afda840dcdd3968ca21c62b1cdbcbc4044e6771b016fe166791ca18a"

	BeforeEach(func() {
		path = filepath.Join("/tmp", "sha-test", strconv.Itoa(GinkgoParallelNode()))
		var err error
		err = os.MkdirAll(path, 0777)
		Expect(err).NotTo(HaveOccurred())
		accessTime = time.Date(1994, time.January, 10, 20, 30, 30, 0, time.UTC)
		modifiedTime := time.Date(1966, time.February, 8, 3, 43, 2, 0, time.UTC)
		Expect(os.Chtimes(path, accessTime, modifiedTime)).To(Succeed())

		ider = repository_fetcher.SHA256{
			KeyFunc: func(path string, timestamp time.Time) string {
				year := timestamp.Year()
				switch year {
				case 1975:
					return "Beckham"
				case 1966:
					return "Stoichkov"
				}

				Fail(fmt.Sprintf("Unexpected year: %d", year))
				return ""
			},
		}
	})

	AfterEach(func() {
		if path != "" {
			Expect(os.RemoveAll(path)).To(Succeed())
		}
	})

	It("returns the hex-converted SHA256 sum of the path", func() {
		hash, err := ider.ID(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(hash).To(Equal(pathHash))
	})

	Context("when the modified time changes", func() {
		const newPathHash = "77e34fc5be0fc17fea6c22dfbb04440bebca41862d0de4b6da0f64396156d0f6"

		BeforeEach(func() {
			Expect(newPathHash).NotTo(Equal(pathHash))
			modifiedTime := time.Date(1975, time.May, 2, 3, 43, 2, 0, time.UTC)
			os.Chtimes(path, accessTime, modifiedTime)
		})

		It("returns a distinct hash", func() {
			hash, err := ider.ID(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(Equal(newPathHash))
		})
	})

	It("returns a string of length 64", func() { // docker verifies this
		hash, err := ider.ID(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(hash).To(HaveLen(64))
	})

	Context("when using a symlink", func() {
		var symlink string
		BeforeEach(func() {
			symlink = filepath.Join("/tmp", fmt.Sprintf("sha-test-symlink-%d", GinkgoParallelNode()))
			Expect(os.Symlink(path, symlink)).To(Succeed())
		})

		AfterEach(func() {
			if symlink != "" {
				Expect(os.Remove(symlink)).To(Succeed())
			}
		})

		It("returns the hash of the target directory", func() {
			hash, err := ider.ID(symlink)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(Equal(pathHash))
		})
	})

	Context("when path does not exist", func() {
		BeforeEach(func() {
			path = "/some/dummy/path/that/does/not/exist"
		})

		It("returns an error", func() {
			_, err := ider.ID(path)
			Expect(err).To(MatchError("repository_fetcher: stat file: lstat /some/dummy/path/that/does/not/exist: no such file or directory"))
		})
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
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "tmp-dir")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		It("registers the image in the graph", func() {
			var registeredImage *image.Image
			fakeGraph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
				registeredImage = image
				return nil
			}

			dirPath := path.Join(tmpDir, "foo/bar/baz")
			err := os.MkdirAll(dirPath, 0700)
			Expect(err).NotTo(HaveOccurred())

			_, _, _, err = fetcher.Fetch(logger, &url.URL{Path: dirPath}, "")
			Expect(err).NotTo(HaveOccurred())

			Expect(registeredImage).NotTo(BeNil())
			Expect(registeredImage.ID).To(HaveSuffix("foo_bar_baz"))
		})

		It("returns a wrapped error if registering fails", func() {
			fakeGraph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
				return errors.New("sold out")
			}

			_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: tmpDir}, "")
			Expect(err).To(MatchError("repository_fetcher: fetch local rootfs: register rootfs: sold out"))
		})

		It("returns the image id", func() {
			dirPath := path.Join(tmpDir, "foo/bar/baz")
			err := os.MkdirAll(dirPath, 0700)
			Expect(err).NotTo(HaveOccurred())

			id, _, _, err := fetcher.Fetch(logger, &url.URL{Path: dirPath}, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(HaveSuffix("foo_bar_baz"))
		})

		It("registers the image with the correct layer data", func() {
			fakeGraph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
				tmp, err := ioutil.TempDir("", "")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmp)

				Expect(archive.Untar(layer, tmp, nil)).To(Succeed())
				Expect(path.Join(tmp, "a", "test", "file")).To(BeAnExistingFile())

				return nil
			}

			tmp, err := ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			Expect(os.MkdirAll(path.Join(tmp, "a", "test"), 0700)).To(Succeed())
			Expect(ioutil.WriteFile(path.Join(tmp, "a", "test", "file"), []byte(""), 0700)).To(Succeed())

			_, _, _, err = fetcher.Fetch(logger, &url.URL{Path: tmp}, "")
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the path is a symlink", func() {
			It("registers the image with the correct layer data", func() {
				fakeGraph.WhenRegistering = func(image *image.Image, layer archive.ArchiveReader) error {
					tmp, err := ioutil.TempDir("", "")
					Expect(err).NotTo(HaveOccurred())
					defer os.RemoveAll(tmp)

					Expect(archive.Untar(layer, tmp, nil)).To(Succeed())
					Expect(path.Join(tmp, "a", "test", "file")).To(BeAnExistingFile())
					return nil
				}

				tmp, err := ioutil.TempDir("", "")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmp)

				tmp2, err := ioutil.TempDir("", "")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmp2)

				symlinkDir := path.Join(tmp2, "rootfs")
				err = os.Symlink(tmp, symlinkDir)
				Expect(err).NotTo(HaveOccurred())

				Expect(os.MkdirAll(path.Join(tmp, "a", "test"), 0700)).To(Succeed())
				Expect(ioutil.WriteFile(path.Join(tmp, "a", "test", "file"), []byte(""), 0700)).To(Succeed())

				_, _, _, err = fetcher.Fetch(logger, &url.URL{Path: symlinkDir}, "")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

type UnderscoreIDer struct{}

func (UnderscoreIDer) ID(path string) (string, error) {
	return strings.Replace(path, "/", "_", -1), nil
}
