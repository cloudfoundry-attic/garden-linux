package quota_manager_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/linux_container/quota_manager"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("AufsQuotaManager", func() {
	var (
		quotaManager quota_manager.AUFSQuotaManager
		mountDir     string
		backingFile  *os.File
		//loopDevice   string
		logger lager.Logger
	)

	BeforeEach(func() {
		quotaManager = quota_manager.AUFSQuotaManager{}
		logger = lagertest.NewTestLogger("AUFSQuotaManager-test")

		var err error
		mountDir, err = ioutil.TempDir("", "quota_manager_test")
		Expect(err).NotTo(HaveOccurred())

		backingFile, err = ioutil.TempFile("", "quota_manager_backing_store")
		Expect(err).NotTo(HaveOccurred())

		Eventually(gexec.Start(exec.Command("truncate", "-s", "10M", backingFile.Name()), GinkgoWriter, GinkgoWriter)).Should(gexec.Exit(0))
		Eventually(gexec.Start(exec.Command("mkfs.ext4", "-F", backingFile.Name()), GinkgoWriter, GinkgoWriter)).Should(gexec.Exit(0))
		Eventually(gexec.Start(exec.Command("mount", "-o", "loop", backingFile.Name(), mountDir), GinkgoWriter, GinkgoWriter)).Should(gexec.Exit(0))
	})

	AfterEach(func() {
		Eventually(gexec.Start(exec.Command("umount", mountDir), GinkgoWriter, GinkgoWriter)).Should(gexec.Exit(0))

		Expect(os.RemoveAll(mountDir)).To(Succeed())
		Expect(os.RemoveAll(backingFile.Name())).To(Succeed())
	})

	Context("when the directory does not match the mount-point of a mounted FS", func() {
		It("returns an error", func() {
			_, err := quotaManager.GetUsage(logger, "does not exist")
			Expect(err).To(MatchError(ContainSubstring("does not exist")))
		})
	})

	Context("when the directory does match the mount-point of an initially empty mounted FS", func() {
		var initialUsage uint64

		BeforeEach(func() {
			stats, err := quotaManager.GetUsage(logger, mountDir)
			Expect(err).NotTo(HaveOccurred())
			initialUsage = stats.ExclusiveBytesUsed
		})

		It("returns stats with ExclusiveBytesUsed close to 0", func() {
			Expect(initialUsage).To(BeNumerically("<", 1024*1024))
		})

		Context("when we write stuff to the FS", func() {
			It("returns stats with accurate ExclusiveBytesUsed", func() {
				Eventually(
					gexec.Start(exec.Command("dd", "if=/dev/zero", fmt.Sprintf("of=%s/some-file", mountDir), "bs=1M", "count=3"), GinkgoWriter, GinkgoWriter),
				).Should(gexec.Exit(0))

				stats, err := quotaManager.GetUsage(logger, mountDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(stats.ExclusiveBytesUsed).Should(BeNumerically("~", initialUsage+3*1024*1024, 512))
			})
		})
	})

	//BeforeEach(func() {
	//	quotaManager = quota_manager.AUFSQuotaManager{}
	//	logger = lagertest.NewTestLogger("AUFSQuotaManager-test")

	//	var err error
	//	mountDir, err = ioutil.TempDir("", "quota_manager_test")
	//	Expect(err).NotTo(HaveOccurred())

	//	backingFile, err = ioutil.TempFile("", "quota_manager_backing_store")
	//	Expect(err).NotTo(HaveOccurred())

	//	cmd := exec.Command("truncate", "-s", "10M", backingFile.Name())
	//	cmd.Stderr = GinkgoWriter
	//	cmd.Stdout = GinkgoWriter
	//	Expect(cmd.Run()).To(Succeed())

	//	cmd = exec.Command("losetup", "-f")
	//	loopDirBytes := gbytes.NewBuffer()
	//	cmd.Stdout = io.MultiWriter(loopDirBytes, GinkgoWriter)
	//	cmd.Stderr = GinkgoWriter
	//	Expect(cmd.Run()).To(Succeed())
	//	loopDevice = strings.TrimSpace(string(loopDirBytes.Contents()))

	//	fmt.Printf("losetup %s %s\n", loopDevice, backingFile.Name())

	//	cmd = exec.Command("losetup", loopDevice, backingFile.Name())
	//	cmd.Stderr = GinkgoWriter
	//	cmd.Stdout = GinkgoWriter
	//	Expect(cmd.Run()).To(Succeed())

	//	cmd = exec.Command("mkfs.ext4", loopDevice)
	//	cmd.Stderr = GinkgoWriter
	//	cmd.Stdout = GinkgoWriter
	//	Expect(cmd.Run()).To(Succeed())

	//	cmd = exec.Command("mount", loopDevice, mountDir)
	//	cmd.Stderr = GinkgoWriter
	//	cmd.Stdout = GinkgoWriter
	//	Expect(cmd.Run()).To(Succeed())
	//})

	//AfterEach(func() {
	//	cmd := exec.Command("umount", mountDir)
	//	cmd.Stderr = GinkgoWriter
	//	cmd.Stdout = GinkgoWriter
	//	Expect(cmd.Run()).To(Succeed())

	//	cmd = exec.Command("losetup", "-d", loopDevice)
	//	cmd.Stderr = GinkgoWriter
	//	cmd.Stdout = GinkgoWriter
	//	Expect(cmd.Run()).To(Succeed())

	//	os.RemoveAll(mountDir)
	//	os.RemoveAll(backingFile.Name())
	//})

	//It("should correctly report the usage of an unused container", func() {
	//	stats, err := quotaManager.GetUsage(logger, mountDir)
	//	Expect(err).NotTo(HaveOccurred())
	//	// There is some overhead for the ext4 filesystem
	//	Expect(stats.ExclusiveBytesUsed).To(BeNumerically("<", 1024*1024))
	//})

	//It("should correctly report the usage of a container we write things into", func() {
	//	stats, err := quotaManager.GetUsage(logger, mountDir)
	//	Expect(err).NotTo(HaveOccurred())

	//	initialUsage := stats.ExclusiveBytesUsed

	//	cmd := exec.Command("dd", "if=/dev/zero", fmt.Sprintf("of=%s/file-alpha", mountDir), "bs=1M", "count=3")
	//	cmd.Stderr = GinkgoWriter
	//	cmd.Stdout = GinkgoWriter
	//	Expect(cmd.Run()).To(Succeed())

	//	stats, err = quotaManager.GetUsage(logger, mountDir)
	//	Expect(err).NotTo(HaveOccurred())
	//	Expect(stats.ExclusiveBytesUsed).Should(BeNumerically("~", initialUsage+3*1024*1024, 512))
	//})

})
