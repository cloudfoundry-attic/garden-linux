package repository_fetcher_test

import (
	"errors"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_cake"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("LayerIDProvider", func() {
	var path1, path2 string
	var accessTime time.Time
	var idp repository_fetcher.LayerIDProvider
	var modifiedTime time.Time

	BeforeEach(func() {
		var err error
		path1, err = ioutil.TempDir("", "sha-test")
		Expect(err).NotTo(HaveOccurred())
		path2, err = ioutil.TempDir("", "sha-test-changed")
		Expect(err).NotTo(HaveOccurred())

		accessTime = time.Date(1994, time.January, 10, 20, 30, 30, 0, time.UTC)
		modifiedTime = time.Date(1966, time.February, 8, 3, 43, 2, 0, time.UTC)
		Expect(os.Chtimes(path1, accessTime, modifiedTime)).To(Succeed())
		Expect(os.Chtimes(path2, accessTime, modifiedTime)).To(Succeed())

		idp = repository_fetcher.LayerIDProvider{}
	})

	AfterEach(func() {
		if path1 != "" {
			Expect(os.RemoveAll(path1)).To(Succeed())
		}
		if path2 != "" {
			Expect(os.RemoveAll(path2)).To(Succeed())
		}
	})

	It("consistently returns the same ID when neither modification time nor path have changed", func() {
		Expect(idp.ProvideID(path1)).To(Equal(idp.ProvideID(path1)))
	})

	It("returns a different ID if the path changes", func() {
		Expect(idp.ProvideID(path1)).NotTo(Equal(idp.ProvideID(path2)))
	})

	It("returns a different ID if the modification time changes", func() {
		beforeID := idp.ProvideID(path1)
		Expect(os.Chtimes(path1, accessTime, modifiedTime.Add(time.Second*1))).To(Succeed())
		Expect(idp.ProvideID(path1)).NotTo(Equal(beforeID))
	})

	Context("when path does not exist", func() {
		BeforeEach(func() {
			path1 = "/some/dummy/path/that/does/not/exist"
		})

	})
})

var _ = Describe("Local", func() {
	var fetcher *repository_fetcher.Local
	var fakeCake *fake_cake.FakeCake
	var defaultRootFSPath string
	var logger lager.Logger

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("local")
		fakeCake = new(fake_cake.FakeCake)
		defaultRootFSPath = ""

		// default to not containing an image
		fakeCake.GetReturns(nil, errors.New("no image"))
	})

	JustBeforeEach(func() {
		fetcher = &repository_fetcher.Local{
			Cake:              fakeCake,
			IDProvider:        UnderscoreIDer{},
			DefaultRootFSPath: defaultRootFSPath,
		}
	})

	Context("when the image already exists in the graph", func() {
		It("returns the image id", func() {
			fakeCake.GetReturns(&image.Image{}, nil)

			rootFSPath, err := ioutil.TempDir("", "testdir")
			Expect(err).NotTo(HaveOccurred())

			rootFSPath = path.Join(rootFSPath, "foo_bar_baz")
			Expect(os.MkdirAll(rootFSPath, 0600)).To(Succeed())

			id, _, _, err := fetcher.Fetch(logger, &url.URL{Path: rootFSPath}, "", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(HaveSuffix("foo_bar_baz"))
		})

		Context("when the path is empty", func() {
			Context("and a default was specified", func() {
				BeforeEach(func() {
					var err error
					defaultRootFSPath, err = ioutil.TempDir("", "default-path")
					Expect(err).NotTo(HaveOccurred())

					defaultRootFSPath = path.Join(defaultRootFSPath, "the_default_path")
					Expect(os.MkdirAll(defaultRootFSPath, 0600)).To(Succeed())
				})

				It("should use the default", func() {
					fakeCake.GetReturns(&image.Image{}, nil)

					id, _, _, err := fetcher.Fetch(logger, &url.URL{Path: ""}, "", 0)
					Expect(err).NotTo(HaveOccurred())
					Expect(id).To(HaveSuffix("the_default_path"))
				})
			})

			Context("and a default was not specified", func() {
				It("should throw an appropriate error", func() {
					_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: ""}, "", 0)
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
			fakeCake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
				registeredImage = image
				return nil
			}

			dirPath := path.Join(tmpDir, "foo/bar/baz")
			err := os.MkdirAll(dirPath, 0700)
			Expect(err).NotTo(HaveOccurred())

			_, _, _, err = fetcher.Fetch(logger, &url.URL{Path: dirPath}, "", 0)
			Expect(err).NotTo(HaveOccurred())

			Expect(registeredImage).NotTo(BeNil())
			Expect(registeredImage.ID).To(HaveSuffix("foo_bar_baz"))
		})

		It("returns a wrapped error if registering fails", func() {
			fakeCake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
				return errors.New("sold out")
			}

			_, _, _, err := fetcher.Fetch(logger, &url.URL{Path: tmpDir}, "", 0)
			Expect(err).To(MatchError("repository_fetcher: fetch local rootfs: register rootfs: sold out"))
		})

		It("returns the image id", func() {
			dirPath := path.Join(tmpDir, "foo/bar/baz")
			err := os.MkdirAll(dirPath, 0700)
			Expect(err).NotTo(HaveOccurred())

			id, _, _, err := fetcher.Fetch(logger, &url.URL{Path: dirPath}, "", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(HaveSuffix("foo_bar_baz"))
		})

		It("registers the image with the correct layer data", func() {
			fakeCake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
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

			_, _, _, err = fetcher.Fetch(logger, &url.URL{Path: tmp}, "", 0)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the path is a symlink", func() {
			It("registers the image with the correct layer data", func() {
				fakeCake.RegisterStub = func(image *image.Image, layer archive.ArchiveReader) error {
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

				_, _, _, err = fetcher.Fetch(logger, &url.URL{Path: symlinkDir}, "", 0)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

type UnderscoreIDer struct{}

func (UnderscoreIDer) ProvideID(path string) layercake.IDer {
	return layercake.DockerImageID(strings.Replace(path, "/", "_", -1))
}
