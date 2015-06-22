package rootfs_provider_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
)

var _ = Describe("Namespacer", func() {
	var rootfs string

	BeforeEach(func() {
		var err error
		rootfs, err = ioutil.TempDir("", "rootfs")
		Expect(err).NotTo(HaveOccurred())

		os.MkdirAll(filepath.Join(rootfs, "foo", "bar", "baz"), 0755)
		ioutil.WriteFile(filepath.Join(rootfs, "foo", "beans"), []byte("jam"), 0755)
	})

	It("translate the root directory", func() {
		var translated []translation
		namespacer := &rootfs_provider.UidNamespacer{
			Logger: lager.NewLogger("test"),
			Translator: func(path string, info os.FileInfo, err error) error {
				translated = append(translated, translation{
					path:    path,
					size:    info.Size(),
					mode:    info.Mode(),
					modTime: info.ModTime(),
					err:     err,
				})

				return nil
			},
		}

		err := namespacer.Namespace(rootfs)

		info, err := os.Stat(rootfs)
		Expect(err).NotTo(HaveOccurred())

		Expect(translated).To(ContainElement(translation{
			path:    rootfs,
			size:    info.Size(),
			mode:    info.Mode(),
			modTime: info.ModTime(),
			err:     nil,
		}))
	})

	It("translates all of the uids", func() {
		var translated []translation
		namespacer := &rootfs_provider.UidNamespacer{
			Logger: lager.NewLogger("test"),
			Translator: func(path string, info os.FileInfo, err error) error {
				translated = append(translated, translation{
					path:    path,
					size:    info.Size(),
					mode:    info.Mode(),
					modTime: info.ModTime(),
					err:     err,
				})

				return nil
			},
		}

		err := namespacer.Namespace(rootfs)

		info, err := os.Stat(filepath.Join(rootfs, "foo", "bar", "baz"))
		Expect(err).NotTo(HaveOccurred())
		Expect(translated).To(ContainElement(translation{
			path:    filepath.Join(rootfs, "foo", "bar", "baz"),
			size:    info.Size(),
			mode:    info.Mode(),
			modTime: info.ModTime(),
			err:     nil,
		}))

		info, err = os.Stat(filepath.Join(rootfs, "foo", "beans"))
		Expect(err).NotTo(HaveOccurred())
		Expect(translated).To(ContainElement(translation{
			path:    filepath.Join(rootfs, "foo", "beans"),
			size:    info.Size(),
			mode:    info.Mode(),
			modTime: info.ModTime(),
			err:     nil,
		}))

		Expect(info.Mode()).To(Equal(os.FileMode(0755)))
	})
})

type translation struct {
	path    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	err     error
}
