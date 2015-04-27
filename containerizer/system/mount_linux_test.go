package system_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Mount", func() {
	var dest string

	BeforeEach(func() {
		var err error
		dest, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(dest)).To(Succeed())
	})

	var testMount = func(privileged bool) {
		Context("with an invalid mount type", func() {
			It("returns an informative error", func() {
				stderr := gbytes.NewBuffer()
				Expect(
					runInContainer(GinkgoWriter, io.MultiWriter(stderr, GinkgoWriter),
						privileged, "fake_mounter", "not-a-mount-type", dest, "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stderr).To(gbytes.Say("error: mount not-a-mount-type on %s: no such device", dest))
			})
		})

		It("can mount tmpfs", func() {
			stdout := gbytes.NewBuffer()
			Expect(
				runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
					privileged, "fake_mounter", string(system.Tmpfs), dest, "cat", "/proc/mounts"),
			).To(Succeed())

			Expect(stdout).To(gbytes.Say(fmt.Sprintf("tmpfs %s tmpfs", dest)))
		})

		Context("when the destination does not already exist", func() {
			It("creates the directory before mounting", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						privileged, "fake_mounter", string(system.Tmpfs), filepath.Join(dest, "foo"), "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stdout).To(gbytes.Say(fmt.Sprintf("tmpfs %s/foo tmpfs", dest)))
			})
		})

		Context("when the destination cannot be created", func() {
			It("returns an informative error", func() {
				ioutil.WriteFile(filepath.Join(dest, "foo"), []byte("block"), 0700)
				stderr := gbytes.NewBuffer()
				Expect(
					runInContainer(GinkgoWriter, io.MultiWriter(stderr, GinkgoWriter),
						privileged, "fake_mounter", "tmpfs", filepath.Join(dest, "foo"), "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stderr).To(gbytes.Say("error: mount: create mount point directory %s/foo: ", dest))
			})
		})
	}

	Context("in an unprivileged container", func() {
		testMount(false)
	})

	Context("in an privileged container", func() {
		testMount(true)
	})
})
