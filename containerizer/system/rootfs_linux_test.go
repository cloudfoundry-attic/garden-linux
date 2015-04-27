package system_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("RootfsLinux", func() {
	It("pivots in to a given rootfs in a privileged container", func() {
		rootfsDir, err := ioutil.TempDir("", "rootfs")
		Expect(err).ToNot(HaveOccurred())
		defer os.RemoveAll(rootfsDir)

		Expect(ioutil.WriteFile(path.Join(rootfsDir, "potato"), []byte{}, 0755)).To(Succeed())

		stdout := gbytes.NewBuffer()
		Expect(runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter, true, "fake_container", rootfsDir)).To(Succeed())
		Expect(stdout).ToNot(gbytes.Say("home"))
		Expect(stdout).To(gbytes.Say("potato"))
	})

	It("pivots in to a given rootfs in an unprivileged container", func() {
		rootfsDir, err := ioutil.TempDir("", "rootfs")
		Expect(err).ToNot(HaveOccurred())
		defer os.RemoveAll(rootfsDir)

		Expect(ioutil.WriteFile(path.Join(rootfsDir, "potato"), []byte{}, 0755)).To(Succeed())

		stdout := gbytes.NewBuffer()
		Expect(runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter, false, "fake_container", rootfsDir)).To(Succeed())
		Expect(stdout).ToNot(gbytes.Say("home"))
		Expect(stdout).To(gbytes.Say("potato"))
	})

	It("unmounts the old rootfs", func() {
		rootfsDir, err := ioutil.TempDir("", "rootfs")
		Expect(err).ToNot(HaveOccurred())
		defer os.RemoveAll(rootfsDir)

		stdout := gbytes.NewBuffer()
		Expect(runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter, false, "fake_container", rootfsDir)).To(Succeed())
		Expect(stdout).ToNot(gbytes.Say("oldroot"))
	})

	It("returns an error when the rootfs does not exist", func() {
		stderr := gbytes.NewBuffer()
		err := runInContainer(GinkgoWriter, io.MultiWriter(stderr, GinkgoWriter), false, "fake_container", "does-not-exist-blah-blah")
		Expect(err).To(HaveOccurred())
		Expect(stderr).To(gbytes.Say("ERROR: Failed to enter root fs: containerizer: validate root file system: stat does-not-exist-blah-blah: no such file or directory"))
	})

	It("returns an error when the rootfs is not a directory", func() {
		tmpFile, err := ioutil.TempFile(os.TempDir(), "rootfs")
		Expect(err).ToNot(HaveOccurred())
		tmpFile.Close()

		defer os.Remove(tmpFile.Name())

		stderr := gbytes.NewBuffer()
		err = runInContainer(GinkgoWriter, io.MultiWriter(stderr, GinkgoWriter), false, "fake_container", tmpFile.Name())
		Expect(err).To(HaveOccurred())
		Expect(stderr).To(gbytes.Say(fmt.Sprintf("ERROR: Failed to enter root fs: containerizer: validate root file system: %s is not a directory", tmpFile.Name())))
	})
})
