package rootfs_provider_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/rootfs_provider"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SimpleVolumeCreator", func() {
	var creator rootfs_provider.SimpleVolumeCreator

	Context("the directory does not already exist in the image", func() {
		It("creates all directories recursively", func() {
			tmpdir, err := ioutil.TempDir("", "volumecreator")
			Ω(err).ShouldNot(HaveOccurred())
			defer os.RemoveAll(tmpdir)

			Ω(creator.Create(tmpdir, filepath.Join("foo", "bar"))).Should(Succeed())

			_, err = os.Stat(filepath.Join(tmpdir, "foo", "bar"))
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("gives it the right permissions (0755)", func() {
			tmpdir, err := ioutil.TempDir("", "volumecreator")
			Ω(err).ShouldNot(HaveOccurred())
			defer os.RemoveAll(tmpdir)

			Ω(creator.Create(tmpdir, filepath.Join("foo", "bar"))).Should(Succeed())

			info, err := os.Stat(filepath.Join(tmpdir, "foo", "bar"))
			Ω(err).ShouldNot(HaveOccurred())
			Ω(info.Mode().Perm()).Should(Equal(os.FileMode(0755)))
		})
	})

	Context("a file with the same name as the directory already exists", func() {
		It("returns an error", func() {
			tmpdir, err := ioutil.TempDir("", "volumecreator")
			Ω(err).ShouldNot(HaveOccurred())
			defer os.RemoveAll(tmpdir)

			volumepath := filepath.Join(tmpdir, "the", "volume")
			Ω(os.MkdirAll(filepath.Join(tmpdir, "the"), 0755)).Should(Succeed())
			Ω(ioutil.WriteFile(volumepath, []byte("beans"), 0755)).Should(Succeed())

			Ω(creator.Create(tmpdir, filepath.Join("/the", "volume"))).ShouldNot(Succeed())
		})
	})

	Context("the directory already exists in the image", func() {
		It("leaves it alone", func() {
			tmpdir, err := ioutil.TempDir("", "volumecreator")
			Ω(err).ShouldNot(HaveOccurred())
			defer os.RemoveAll(tmpdir)

			volumepath := filepath.Join(tmpdir, "the", "volume")
			Ω(os.MkdirAll(volumepath, 0755)).Should(Succeed())

			Ω(ioutil.WriteFile(filepath.Join(volumepath, "foo.txt"), []byte("beans"), 0755)).Should(Succeed())

			Ω(creator.Create(tmpdir, filepath.Join("the", "volume"))).Should(Succeed())

			_, err = os.Stat(volumepath)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(filepath.Glob(filepath.Join(volumepath, "*"))).Should(ContainElement(filepath.Join(volumepath, "foo.txt")))
		})
	})
})
