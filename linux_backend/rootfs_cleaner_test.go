package linux_backend_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/garden-linux/linux_backend"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
)

var _ = Describe("RootfsCleaner", func() {
	var (
		filePaths []string

		cleaner  *linux_backend.RootFSCleaner
		log      lager.Logger
		rootPath string
	)

	BeforeEach(func() {
		var err error

		filePaths = []string{}

		log = lagertest.NewTestLogger("test")
		rootPath, err = ioutil.TempDir("", "")
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		cleaner = &linux_backend.RootFSCleaner{
			FilePaths: filePaths,
		}
	})

	AfterEach(func() {
		Expect(os.RemoveAll(rootPath)).To(Succeed())
	})

	Describe("Clean", func() {
		createFile := func(filePath string) {
			Expect(os.MkdirAll(filepath.Dir(filePath), 0777)).To(Succeed())
			f, err := os.Create(filePath)
			defer f.Close()
			Expect(err).NotTo(HaveOccurred())
		}

		createSymlink := func(srcFilePath, destFilePath string) {
			Expect(os.MkdirAll(filepath.Dir(srcFilePath), 0777)).To(Succeed())
			Expect(os.Symlink(destFilePath, srcFilePath)).To(Succeed())
		}

		createTempFile := func() string {
			destFile, err := ioutil.TempFile("", "")
			defer destFile.Close()
			Expect(err).NotTo(HaveOccurred())
			return destFile.Name()
		}

		Context("when the list is empty", func() {
			It("should succeed", func() {
				Expect(cleaner.Clean(log, rootPath)).To(Succeed())
			})
		})

		Context("when there is a single path", func() {
			BeforeEach(func() {
				filePaths = append(filePaths, "/etc/config")
			})

			Context("and it does not exist in the root path", func() {
				It("should succeed", func() {
					Expect(cleaner.Clean(log, rootPath)).To(Succeed())
				})
			})

			Context("and it exists in the root path", func() {
				Context("and it is a symlink", func() {
					var destFilePath string

					BeforeEach(func() {
						destFilePath = createTempFile()

						createSymlink(filepath.Join(rootPath, "/etc/config"), destFilePath)
					})

					AfterEach(func() {
						Expect(os.Remove(destFilePath)).To(Succeed())
					})

					It("should remove it", func() {
						Expect(cleaner.Clean(log, rootPath)).To(Succeed())

						Expect(filepath.Join(rootPath, "/etc/config")).NotTo(BeAnExistingFile())
					})
				})

				Context("and it is not a symlink", func() {
					BeforeEach(func() {
						createFile(filepath.Join(rootPath, "/etc/config"))
					})

					It("should not touch it", func() {
						Expect(cleaner.Clean(log, rootPath)).To(Succeed())

						Expect(filepath.Join(rootPath, "/etc/config")).To(BeAnExistingFile())
					})
				})
			})
		})

		Context("when there are multiple paths", func() {
			var (
				configDestFile    string
				nosymlinkDestFile string
			)

			BeforeEach(func() {
				filePaths = append(
					filePaths,
					"/etc/config",
					"/a_root_file",
					"/var/no_symlink",
					"/home/alice/in_wonderland",
				)

				configDestFile = createTempFile()
				createSymlink(filepath.Join(rootPath, "/etc/config"), configDestFile)
				nosymlinkDestFile = createTempFile()
				createSymlink(filepath.Join(rootPath, "/var/no_symlink"), nosymlinkDestFile)
				createFile(filepath.Join(rootPath, "/home/alice/in_wonderland"))
			})

			AfterEach(func() {
				Expect(os.Remove(configDestFile)).To(Succeed())
				Expect(os.Remove(nosymlinkDestFile)).To(Succeed())
			})

			It("should delete the symlinks", func() {
				Expect(cleaner.Clean(log, rootPath)).To(Succeed())

				Expect(filepath.Join(rootPath, "/etc/config")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(rootPath, "/var/no_symlink")).NotTo(BeAnExistingFile())
			})

			It("should not touch the existing files that are not symlinks", func() {
				Expect(cleaner.Clean(log, rootPath)).To(Succeed())

				Expect(filepath.Join(rootPath, "/a_root_file")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(rootPath, "/home/alice/in_wonderland")).To(BeAnExistingFile())
			})
		})
	})
})
