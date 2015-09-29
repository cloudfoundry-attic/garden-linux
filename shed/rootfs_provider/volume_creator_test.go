package rootfs_provider_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/shed/rootfs_provider"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SimpleVolumeCreator", func() {
	var creator rootfs_provider.SimpleVolumeCreator

	Context("the directory does not already exist in the image", func() {
		It("creates all directories recursively", func() {
			tmpdir, err := ioutil.TempDir("", "volumecreator")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tmpdir)

			Expect(creator.Create(tmpdir, filepath.Join("foo", "bar"))).To(Succeed())

			_, err = os.Stat(filepath.Join(tmpdir, "foo", "bar"))
			Expect(err).ToNot(HaveOccurred())
		})

		It("gives it the right permissions (0755)", func() {
			tmpdir, err := ioutil.TempDir("", "volumecreator")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tmpdir)

			Expect(creator.Create(tmpdir, filepath.Join("foo", "bar"))).To(Succeed())

			info, err := os.Stat(filepath.Join(tmpdir, "foo", "bar"))
			Expect(err).ToNot(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0755)))
		})
	})

	Context("a file with the same name as the directory already exists", func() {
		It("returns an error", func() {
			tmpdir, err := ioutil.TempDir("", "volumecreator")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tmpdir)

			volumepath := filepath.Join(tmpdir, "the", "volume")
			Expect(os.MkdirAll(filepath.Join(tmpdir, "the"), 0755)).To(Succeed())
			Expect(ioutil.WriteFile(volumepath, []byte("beans"), 0755)).To(Succeed())

			Expect(creator.Create(tmpdir, filepath.Join("/the", "volume"))).ToNot(Succeed())
		})
	})

	Context("the directory already exists in the image", func() {
		It("leaves it alone", func() {
			tmpdir, err := ioutil.TempDir("", "volumecreator")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tmpdir)

			volumepath := filepath.Join(tmpdir, "the", "volume")
			Expect(os.MkdirAll(volumepath, 0755)).To(Succeed())

			Expect(ioutil.WriteFile(filepath.Join(volumepath, "foo.txt"), []byte("beans"), 0755)).To(Succeed())

			Expect(creator.Create(tmpdir, filepath.Join("the", "volume"))).To(Succeed())

			_, err = os.Stat(volumepath)
			Expect(err).ToNot(HaveOccurred())

			Expect(filepath.Glob(filepath.Join(volumepath, "*"))).To(ContainElement(filepath.Join(volumepath, "foo.txt")))
		})
	})
})
