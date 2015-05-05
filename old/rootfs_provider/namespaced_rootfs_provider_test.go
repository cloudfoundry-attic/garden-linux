package rootfs_provider_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider/fake_copier"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
)

var _ = Describe("Namespacer", func() {
	var rootfs string
	var workDir string

	BeforeEach(func() {
		var err error
		rootfs, err = ioutil.TempDir("", "rootfs")
		Expect(err).NotTo(HaveOccurred())

		workDir, err = ioutil.TempDir("", "rootfs-workdir")
		Expect(err).NotTo(HaveOccurred())

		os.MkdirAll(filepath.Join(rootfs, "foo", "bar", "baz"), 0755)
		ioutil.WriteFile(filepath.Join(rootfs, "foo", "beans"), []byte("jam"), 0755)
	})

	It("translates all of the uids in to a copy of the rootfs", func() {
		var translated []translation
		namespacer := &rootfs_provider.UidNamespacer{
			Logger: lager.NewLogger("test"),
			Copier: &rootfs_provider.ShellOutCp{workDir},
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

		err := namespacer.Namespace(rootfs, workDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(translated).NotTo(BeEmpty())

		info, err := os.Stat(filepath.Join(workDir, "foo", "bar", "baz"))
		Expect(err).NotTo(HaveOccurred())
		Expect(translated).To(ContainElement(translation{
			path:    filepath.Join(workDir, "foo", "bar", "baz"),
			size:    info.Size(),
			mode:    info.Mode(),
			modTime: info.ModTime(),
			err:     nil,
		}))

		info, err = os.Stat(filepath.Join(workDir, "foo", "beans"))
		Expect(err).NotTo(HaveOccurred())
		Expect(translated).To(ContainElement(translation{
			path:    filepath.Join(workDir, "foo", "beans"),
			size:    info.Size(),
			mode:    info.Mode(),
			modTime: info.ModTime(),
			err:     nil,
		}))

		Expect(info.Mode()).To(Equal(os.FileMode(0755)))
	})

	Context("when copying the rootfs fails", func() {
		var (
			fakeCopier *fake_copier.FakeCopier
			logger     lager.Logger
		)

		BeforeEach(func() {
			fakeCopier = new(fake_copier.FakeCopier)
			fakeCopier.CopyReturns(errors.New("will not copy"))
			logger = lager.NewLogger("test")
		})

		It("returns an error", func() {
			namespacer := &rootfs_provider.UidNamespacer{
				Logger: logger,
				Copier: fakeCopier,
				Translator: func(path string, info os.FileInfo, err error) error {
					return nil
				},
			}

			err := namespacer.Namespace("whatever", "towherever")
			Expect(err).To(HaveOccurred())
		})

		It("does not run the translation", func() {
			namespacer := &rootfs_provider.UidNamespacer{
				Logger: logger,
				Copier: fakeCopier,
				Translator: func(path string, info os.FileInfo, err error) error {
					Fail("should not be run")
					return nil
				},
			}

			namespacer.Namespace("whatever", "whenever")
		})
	})
})

type translation struct {
	path    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	err     error
}
