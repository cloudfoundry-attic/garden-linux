package system_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("InitializerLinux", func() {
	Describe("Init", func() {
		PIt("asks the network configurer to configure", func() {})

		var root string

		BeforeEach(func() {
			var err error
			root, err = ioutil.TempDir("", "")
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(root)).To(Succeed())
		})

		Context("when /dev/shm already exists", func() {
			BeforeEach(func() {
				Expect(os.MkdirAll(filepath.Join(root, "dev", "shm"), 0700)).To(Succeed())
			})

			It("mounts tmpfs on /dev/shm", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						false, "fake_initializer", root, "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stdout).To(gbytes.Say(fmt.Sprintf("tmpfs %s/dev/shm tmpfs", root)))
			})
		})

		Context("when /dev/shm does not already exist", func() {
			It("creates the directory before mounting tmpfs", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						false, "fake_initializer", root, "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stdout).To(gbytes.Say(fmt.Sprintf("tmpfs %s/dev/shm tmpfs", root)))
			})
		})

		Context("when /proc already exists", func() {
			BeforeEach(func() {
				Expect(os.MkdirAll(filepath.Join(root, "proc"), 0700)).To(Succeed())
			})

			It("mounts proc on /proc", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						false, "fake_initializer", root, "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stdout).To(gbytes.Say(fmt.Sprintf("proc %s/proc proc", root)))
			})
		})

		Context("when /proc does not exist", func() {
			It("mounts proc on /proc", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						false, "fake_initializer", root, "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stdout).To(gbytes.Say(fmt.Sprintf("proc %s/proc proc", root)))
			})
		})
	})
})
