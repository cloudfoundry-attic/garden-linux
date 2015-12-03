package system_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("RootfsLinux", func() {
	var rootfsDir string
	BeforeEach(func() {
		rootfsDir = path.Join(tempDirPath, fmt.Sprintf("rootfs-%d", GinkgoParallelNode()))
		if _, err := os.Stat(rootfsDir); os.IsNotExist(err) {
			Expect(os.MkdirAll(rootfsDir, 0700)).To(Succeed())
		}
	})

	AfterEach(func() {
		entries, err := ioutil.ReadDir(rootfsDir)
		Expect(err).NotTo(HaveOccurred())
		for _, entry := range entries {
			Expect(os.RemoveAll(entry.Name()))
		}

		// Cleaning up this directoy causes the VM to hang occasionally.
		// Expect(os.RemoveAll(rootfsDir)).To(Succeed())
	})

	It("pivots in to a given rootfs in a privileged container", func() {
		Expect(ioutil.WriteFile(path.Join(rootfsDir, "potato"), []byte{}, 0755)).To(Succeed())

		stdout := gbytes.NewBuffer()
		Expect(runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter, true, "fake_container", rootfsDir)).To(Succeed())
		Expect(stdout).ToNot(gbytes.Say("home"))
		Expect(stdout).To(gbytes.Say("potato"))
	})

	It("pivots in to a given rootfs in an unprivileged container", func() {
		Expect(ioutil.WriteFile(path.Join(rootfsDir, "potato"), []byte{}, 0755)).To(Succeed())

		stdout := gbytes.NewBuffer()
		Expect(runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter, false, "fake_container", rootfsDir)).To(Succeed())
		Expect(stdout).ToNot(gbytes.Say("home"))
		Expect(stdout).To(gbytes.Say("potato"))
	})

	Context("when the rootfs contains bind mounts", func() {
		var (
			targetBindMountDir string
			sourceBindMountDir string
		)

		BeforeEach(func() {
			var err error

			targetBindMountDir = filepath.Join(rootfsDir, "a-bind-mount")
			Expect(os.Mkdir(targetBindMountDir, 0755)).To(Succeed())

			sourceBindMountDir, err = ioutil.TempDir("", "bnds")
			Expect(err).ToNot(HaveOccurred())

			Expect(syscall.Mount(sourceBindMountDir, targetBindMountDir, "", uintptr(syscall.MS_BIND), "")).To(Succeed())
		})

		AfterEach(func() {
			Expect(syscall.Unmount(targetBindMountDir, 0)).To(Succeed())

			// Cleaning up this directories causes the VM to hang occasionally.
			// Expect(os.RemoveAll(sourceBindMountDir)).To(Succeed())
			// Expect(os.RemoveAll(targetBindMountDir)).To(Succeed())
		})

		It("pivots in to the given rootfs", func() {
			stdout := gbytes.NewBuffer()
			out := io.MultiWriter(stdout, GinkgoWriter)
			Expect(runInContainer(out, out, false, "fake_container", rootfsDir)).To(Succeed())
			Expect(stdout).ToNot(gbytes.Say("home"))
			Expect(stdout).To(gbytes.Say("a-bind-mount"))
		})
	})

	It("unmounts the old rootfs", func() {
		stdout := gbytes.NewBuffer()
		Expect(runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter, false, "fake_container", rootfsDir)).To(Succeed())
		Expect(stdout).ToNot(gbytes.Say("oldroot"))
	})

	It("returns an error when the rootfs does not exist", func() {
		stderr := gbytes.NewBuffer()
		err := runInContainer(GinkgoWriter, io.MultiWriter(stderr, GinkgoWriter), false, "fake_container", "does-not-exist-blah-blah")
		Expect(err).To(HaveOccurred())
		Expect(stderr).To(gbytes.Say("ERROR: Failed to enter root fs: system: validate root file system: stat does-not-exist-blah-blah: no such file or directory"))
	})

	It("returns an error when the rootfs is not a directory", func() {
		tmpFile, err := ioutil.TempFile(os.TempDir(), "rootfs")
		Expect(err).ToNot(HaveOccurred())
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		stderr := gbytes.NewBuffer()
		err = runInContainer(GinkgoWriter, io.MultiWriter(stderr, GinkgoWriter), false, "fake_container", tmpFile.Name())
		Expect(err).To(HaveOccurred())
		Expect(stderr).To(gbytes.Say(fmt.Sprintf("ERROR: Failed to enter root fs: system: validate root file system: %s is not a directory", tmpFile.Name())))
	})
})
