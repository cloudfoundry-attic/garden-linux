package layercake_test

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/shed/layercake"
	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/graph"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	_ "github.com/docker/docker/daemon/graphdriver/vfs"
	_ "github.com/docker/docker/pkg/chrootarchive" // allow reexec of docker-applyLayer
	"github.com/docker/docker/pkg/reexec"
)

func init() {
	reexec.Init()
}

var _ = Describe("Docker", func() {
	var (
		root string
		cake *layercake.Docker
	)

	BeforeEach(func() {
		var err error
		root, err = ioutil.TempDir("", "cakeroot")
		Expect(err).NotTo(HaveOccurred())

		driver, err := graphdriver.New(root, nil)
		Expect(err).NotTo(HaveOccurred())

		graph, err := graph.NewGraph(root, driver)
		Expect(err).NotTo(HaveOccurred())

		cake = &layercake.Docker{
			Graph: graph,
		}
	})

	Describe("Register", func() {
		Context("after registering a layer", func() {
			var id layercake.ID
			var parent layercake.ID

			BeforeEach(func() {
				id = layercake.ContainerID("")
				parent = layercake.ContainerID("")
			})

			ItCanReadWriteTheLayer := func() {
				It("can read and write files", func() {
					p, err := cake.Path(id)
					Expect(err).NotTo(HaveOccurred())
					Expect(ioutil.WriteFile(path.Join(p, "foo"), []byte("hi"), 0700)).To(Succeed())

					p, err = cake.Path(id)
					Expect(err).NotTo(HaveOccurred())
					Expect(path.Join(p, "foo")).To(BeAnExistingFile())
				})

				It("can get back the image", func() {
					img, err := cake.Get(id)
					Expect(err).NotTo(HaveOccurred())
					Expect(img.ID).To(Equal(id.GraphID()))
					Expect(img.Parent).To(Equal(parent.GraphID()))
				})
			}

			Context("when the new layer is a docker image", func() {
				JustBeforeEach(func() {
					id = layercake.DockerImageID("70d8f0edf5c9008eb61c7c52c458e7e0a831649dbb238b93dde0854faae314a8")
					registerImageLayer(cake, &image.Image{
						ID:     id.GraphID(),
						Parent: parent.GraphID(),
					})
				})

				Context("without a parent", func() {
					ItCanReadWriteTheLayer()

					It("can read the files in the image", func() {
						p, err := cake.Path(id)
						Expect(err).NotTo(HaveOccurred())

						Expect(path.Join(p, id.GraphID())).To(BeAnExistingFile())
					})

					It("can be deleted", func() {
						cake.Remove(id)

						filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
							Expect(path).To(BeADirectory())
							return nil
						})
					})
				})

				Context("with a parent", func() {
					BeforeEach(func() {
						parent = layercake.DockerImageID("07d8fe0df5c9008eb16c7c52c548e7e0a831649dbb238b93dde0854faae3148a")
						registerImageLayer(cake, &image.Image{
							ID:     parent.GraphID(),
							Parent: "",
						})
					})

					ItCanReadWriteTheLayer()

					It("inherits files from the parent layer", func() {
						p, err := cake.Path(id)
						Expect(err).NotTo(HaveOccurred())

						Expect(path.Join(p, parent.GraphID())).To(BeAnExistingFile())
					})

					It("can read the files in the image", func() {
						p, err := cake.Path(id)
						Expect(err).NotTo(HaveOccurred())

						Expect(path.Join(p, id.GraphID())).To(BeAnExistingFile())
					})
				})
			})

			Context("when the new layer is a container", func() {
				Context("with a parent", func() {
					BeforeEach(func() {
						parent = layercake.DockerImageID("70d8f0edf5c9008eb61c7c52c458e7e0a831649dbb238b93dde0854faae314a8")
						registerImageLayer(cake, &image.Image{
							ID:     parent.GraphID(),
							Parent: "",
						})

						id = layercake.ContainerID("abc")
						createContainerLayer(cake, id, parent)
					})

					ItCanReadWriteTheLayer()

					It("inherits files from the parent layer", func() {
						p, err := cake.Path(id)
						Expect(err).NotTo(HaveOccurred())

						Expect(path.Join(p, parent.GraphID())).To(BeAnExistingFile())
					})
				})
			})
		})
	})

	Describe("IsLeaf", func() {
		BeforeEach(func() {
			createContainerLayer(cake, layercake.ContainerID("def"), layercake.DockerImageID(""))
			createContainerLayer(cake, layercake.ContainerID("abc"), layercake.ContainerID("def"))
			createContainerLayer(cake, layercake.ContainerID("child2"), layercake.ContainerID("def"))
		})

		Context("when an image has no children", func() {
			It("is a leaf", func() {
				Expect(cake.IsLeaf(layercake.ContainerID("abc"))).To(Equal(true))
			})
		})

		Context("when an image has children", func() {
			It("is not a leaf", func() {
				Expect(cake.IsLeaf(layercake.ContainerID("def"))).To(Equal(false))
			})
		})

		Context("when an image's final child is removed", func() {
			It("is becomes a leaf", func() {
				Expect(cake.IsLeaf(layercake.ContainerID("def"))).To(Equal(false))

				Expect(cake.Remove(layercake.ContainerID("abc"))).To(Succeed())
				Expect(cake.IsLeaf(layercake.ContainerID("def"))).To(Equal(false))

				Expect(cake.Remove(layercake.ContainerID("child2"))).To(Succeed())
				Expect(cake.IsLeaf(layercake.ContainerID("def"))).To(Equal(true))
			})
		})
	})
})

func createContainerLayer(cake *layercake.Docker, id, parent layercake.ID) {
	Expect(cake.Create(id, parent)).To(Succeed())
}

func registerImageLayer(cake *layercake.Docker, img *image.Image) {
	tmp, err := ioutil.TempDir("", "my-img")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tmp)

	Expect(ioutil.WriteFile(path.Join(tmp, img.ID), []byte("Hello"), 0700)).To(Succeed())
	archiver, _ := archive.Tar(tmp, archive.Uncompressed)

	Expect(cake.Register(img, archiver)).To(Succeed())
}
