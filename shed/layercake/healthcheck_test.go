package layercake_test

import (
	"io/ioutil"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/shed/layercake"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Health Check", func() {
	Context("when the graph directory is read-write", func() {
		var hc layercake.GraphPath
		BeforeEach(func() {
			tmpDir, err := ioutil.TempDir("", "garden-test")
			Expect(err).NotTo(HaveOccurred())
			hc = layercake.GraphPath(tmpDir)
		})

		It("should not return an error", func() {
			Expect(hc.HealthCheck()).To(Succeed())
		})
	})

	Context("when the graph directory is not writable", func() {
		var graphDirFile string
		var hc layercake.GraphPath

		BeforeEach(func() {
			// Try to use a file as the graphdir -- this is certainly not a writable directory.
			tempFile, err := ioutil.TempFile("", "read-only-graph-dir-test")
			Expect(err).NotTo(HaveOccurred())

			graphDirFile = tempFile.Name()
			err = tempFile.Close()
			Expect(err).NotTo(HaveOccurred())

			hc = layercake.GraphPath(tempFile.Name())
		})

		AfterEach(func() {
			os.RemoveAll(graphDirFile)
		})

		It("should return an error", func() {
			err := hc.HealthCheck()
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(MatchRegexp("graph directory '%s' is not writeable:.*", graphDirFile))
		})
	})
})
