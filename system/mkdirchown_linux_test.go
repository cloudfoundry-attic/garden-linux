package system_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"code.cloudfoundry.org/garden-linux/system"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MkdirChown", func() {
	var (
		tmpdir, mypath string
	)

	BeforeEach(func() {
		var err error
		tmpdir, err = ioutil.TempDir("", "mkdirchowner")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpdir)
	})

	Context("when we can make and chown a directory", func() {
		BeforeEach(func() {
			mypath = filepath.Join(tmpdir, "thing")
		})

		JustBeforeEach(func() {
			err := system.MkdirChown(mypath, 12, 32, 0755)
			Expect(err).NotTo(HaveOccurred())
		})

		It("makes the directory", func() {
			Expect(mypath).To(BeADirectory())
		})

		It("gives it the right mode", func() {
			info, err := os.Stat(mypath)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode() & 0755).To(BeEquivalentTo((0755)))
		})

		Context("when the parent of the dir to create doesn't exist", func() {
			BeforeEach(func() {
				mypath = filepath.Join(tmpdir, "my", "box", "of", "things")
			})

			It("makes all the directories", func() {
				Expect(mypath).To(BeADirectory())
			})

			It("chowns all the directories to uid,gid", func() {
				info, err := os.Stat(mypath)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Sys().(*syscall.Stat_t).Uid).To(BeEquivalentTo(12))
				Expect(info.Sys().(*syscall.Stat_t).Gid).To(BeEquivalentTo(32))

				info, err = os.Stat(filepath.Dir(mypath))
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Sys().(*syscall.Stat_t).Uid).To(BeEquivalentTo(12))
				Expect(info.Sys().(*syscall.Stat_t).Gid).To(BeEquivalentTo(32))

				info, err = os.Stat(filepath.Dir(filepath.Dir(mypath)))
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Sys().(*syscall.Stat_t).Uid).To(BeEquivalentTo(12))
				Expect(info.Sys().(*syscall.Stat_t).Gid).To(BeEquivalentTo(32))
			})
		})
	})

	Context("when one of the directories in the stack is impossible to create", func() {
		var (
			tmpfile *os.File
		)

		BeforeEach(func() {
			var err error
			tmpfile, err = ioutil.TempFile(tmpdir, "mkdirchown")
			Expect(err).NotTo(HaveOccurred())
			mypath = filepath.Join(tmpfile.Name(), "my", "box", "of", "things")
		})

		It("returns a sensible error", func() {
			err := system.MkdirChown(mypath, 12, 32, 0755)
			Expect(err).To(MatchError(ContainSubstring("mkdir")))
		})
	})
})
