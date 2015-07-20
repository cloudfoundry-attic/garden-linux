package system_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Mount", func() {
	var (
		dest string
		src  string
	)

	BeforeEach(func() {
		var err error
		src, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		dest = fmt.Sprintf("/tmp/mount-dest-temp-%d", GinkgoParallelNode())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(src)).To(Succeed())
		Expect(os.RemoveAll(dest)).To(Succeed())
	})

	var testMount = func(privileged bool) {
		Context("with an invalid mount type", func() {
			It("returns an informative error", func() {
				stderr := gbytes.NewBuffer()
				Expect(
					runInContainer(GinkgoWriter, io.MultiWriter(stderr, GinkgoWriter),
						privileged, "fake_mounter", "-type=not-a-mount-type", "-targetPath="+dest, "-flags=0", "cat", "/proc/mounts"),
				).To(HaveOccurred())

				Expect(stderr).To(gbytes.Say("error: system: mount not-a-mount-type on %s: no such device", dest))
			})
		})

		It("can mount tmpfs", func() {
			stdout := gbytes.NewBuffer()
			Expect(
				runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
					privileged, "fake_mounter", "-type="+string(system.Tmpfs), "-targetPath="+dest, "-flags=0", "cat", "/proc/mounts"),
			).To(Succeed())

			Expect(stdout).To(gbytes.Say(fmt.Sprintf("tmpfs %s tmpfs", dest)))
		})

		Context("when flags are supplied", func() {
			It("mounts using the flags", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						privileged, "fake_mounter", "-type="+string(system.Tmpfs), "-targetPath="+dest, fmt.Sprintf("-flags=%d", syscall.MS_NODEV), "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stdout).To(gbytes.Say(fmt.Sprintf("tmpfs %s tmpfs rw,nodev", dest)))
			})
		})

		Context("when data is provided", func() {
			It("mounts using the data", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						privileged, "fake_mounter", "-type="+string(system.Devpts), "-targetPath="+dest, "-flags=0", "-data=newinstance,ptmxmode=0666", "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stdout).To(gbytes.Say(fmt.Sprintf("devpts %s devpts rw,relatime,mode=600,ptmxmode=666", dest)))
			})
		})

		Context("when the destination does not already exist", func() {
			It("creates the directory before mounting", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						privileged, "fake_mounter", "-type="+string(system.Tmpfs), "-targetPath="+filepath.Join(dest, "foo"), "-flags=0", "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stdout).To(gbytes.Say(fmt.Sprintf("tmpfs %s/foo tmpfs", dest)))
			})
		})

		Context("when the destination cannot be created", func() {
			BeforeEach(func() {
				var err error
				dest, err = ioutil.TempDir("", "")
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns an informative error", func() {
				ioutil.WriteFile(filepath.Join(dest, "foo"), []byte("block"), 0700)
				stderr := gbytes.NewBuffer()
				Expect(
					runInContainer(GinkgoWriter, io.MultiWriter(stderr, GinkgoWriter),
						privileged, "fake_mounter", "-type=tmpfs", "-targetPath="+filepath.Join(dest, "foo"), "-flags=0", "cat", "/proc/mounts"),
				).To(HaveOccurred())

				Expect(stderr).To(gbytes.Say("error: system: create mount point directory %s/foo: ", dest))
			})
		})

		Context("when the sourcePath is provided", func() {
			It("mounts using the sourcePath", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						privileged, "fake_mounter", "-type=bind", "-sourcePath="+src, "-targetPath="+dest, fmt.Sprintf("-flags=%d", syscall.MS_BIND), "cat", "/proc/mounts"),
				).To(Succeed())

				Expect(stdout).To(gbytes.Say(fmt.Sprintf("%s ext4 rw,relatime,errors=remount-ro,data=ordered", dest)))
			})
		})

		Context("when file is mounted", func() {
			var (
				srcFile string
				dstFile string
			)

			BeforeEach(func() {
				file, err := ioutil.TempFile("", "")
				Expect(err).ToNot(HaveOccurred())
				fmt.Fprintf(file, "MountMe")
				srcFile = file.Name()
				dstFile = fmt.Sprintf("/tmp/fake-mount-file-%d", GinkgoParallelNode())
			})

			AfterEach(func() {
				Expect(os.Remove(srcFile)).To(Succeed())
			})

			It("mounts a file using the sourcePath", func() {
				stdout := gbytes.NewBuffer()
				Expect(
					runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
						privileged, "fake_mounter", "-type=bind", "-sourcePath="+srcFile, "-targetPath="+dstFile, fmt.Sprintf("-flags=%d", syscall.MS_BIND), "cat", "/proc/mounts"),
				).To(Succeed())

				info, err := os.Stat(dstFile)
				Expect(err).ToNot(HaveOccurred())
				Expect(info.IsDir()).ToNot(BeTrue())
				Expect(stdout).To(gbytes.Say(fmt.Sprintf("%s ext4 rw,relatime,errors=remount-ro,data=ordered", dstFile)))
				Expect(os.Remove(dstFile)).To(Succeed())
			})

			Context("when destination file cannot be created", func() {
				It("returns an informative error", func() {
					stderr := gbytes.NewBuffer()
					Expect(
						runInContainer(GinkgoWriter, io.MultiWriter(GinkgoWriter, stderr),
							privileged, "fake_mounter", "-type=bind", "-sourcePath="+srcFile, "-targetPath=/tmp", fmt.Sprintf("-flags=%d", syscall.MS_BIND), "cat", "/proc/mounts"),
					).To(HaveOccurred())

					Expect(stderr).To(gbytes.Say("error: system: create mount point file /tmp: "))
				})
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
