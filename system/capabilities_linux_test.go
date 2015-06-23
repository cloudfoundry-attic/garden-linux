package system_test

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/system"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Capabilities", func() {
	var (
		testPath string
	)

	BeforeEach(func() {
		var err error
		testPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/system/test_capabilities")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		gexec.CleanupBuildArtifacts()
	})

	Context("when extended whitelist is not requested", func() {
		It("limits capabilities to docker whitelist", func() {
			testOut := gbytes.NewBuffer()
			runningTest, err := gexec.Start(
				exec.Command(testPath, "-extendedWhitelist=false"),
				io.MultiWriter(GinkgoWriter, testOut),
				GinkgoWriter,
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				runningTest.Kill()
				Eventually(runningTest).Should(gexec.Exit())
			}()
			Eventually(testOut).Should(gbytes.Say("banana"))

			cmd := exec.Command("cat", fmt.Sprintf("/proc/%d/status", runningTest.Command.Process.Pid))
			catOut := gbytes.NewBuffer()
			cmd.Stdout = io.MultiWriter(GinkgoWriter, catOut)
			cmd.Stderr = io.MultiWriter(GinkgoWriter, catOut)
			Expect(cmd.Run()).To(Succeed())
			Expect(catOut).To(gbytes.Say("CapBnd:	00000000a80425fa"))
		})
	})

	Context("when extended whitelist is requested", func() {
		It("limits capabilities to docker whitelist + CAP_SYS_ADMIN", func() {
			testOut := gbytes.NewBuffer()
			runningTest, err := gexec.Start(
				exec.Command(testPath, "-extendedWhitelist=true"),
				io.MultiWriter(GinkgoWriter, testOut),
				GinkgoWriter,
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				runningTest.Kill()
				Eventually(runningTest).Should(gexec.Exit())
			}()
			Eventually(testOut).Should(gbytes.Say("banana"))

			cmd := exec.Command("cat", fmt.Sprintf("/proc/%d/status", runningTest.Command.Process.Pid))
			catOut := gbytes.NewBuffer()
			cmd.Stdout = io.MultiWriter(GinkgoWriter, catOut)
			cmd.Stderr = io.MultiWriter(GinkgoWriter, catOut)
			Expect(cmd.Run()).To(Succeed())
			Expect(catOut).To(gbytes.Say("CapBnd:	00000000a82425fa"))
		})
	})

	Context("when the pid does not exist", func() {
		It("does not modify capabilties and returns error", func() {
			Expect(system.ProcessCapabilities{Pid: 1000000}.Limit(false)).To(MatchError(ContainSubstring("getting capabilities")))
		})
	})
})
