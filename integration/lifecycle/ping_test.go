package lifecycle_test

import (
	"io/ioutil"

	"os"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Ping", func() {
	Context("When a garden linux server is started", func() {
		BeforeEach(func() {
			client = startGarden()
		})

		It("should successfully ping", func() {
			Expect(client.Ping()).To(Succeed())
		})
	})

	Context("When a garden linux server is started with an untennable graph directory", func() {
		var graphDirFile string

		BeforeEach(func() {
			// Try to use a file as the graphdir -- this is certainly not a writable directory.
			tempFile, err := ioutil.TempFile("", "read-only-graph-dir-test")
			Expect(err).NotTo(HaveOccurred())

			graphDirFile = tempFile.Name()
			err = tempFile.Close()
			Expect(err).NotTo(HaveOccurred())

			client = startGarden("--graph", graphDirFile)
		})

		AfterEach(func() {
			if graphDirFile != "" {
				os.RemoveAll(graphDirFile)
			}
		})

		It("should fail when pinged", func() {
			err := client.Ping()
			Expect(err).NotTo(BeNil())
			Expect(err).To(BeAssignableToTypeOf(garden.NewUnrecoverableError("")))
			Expect(err.Error()).To(MatchRegexp("graph directory '%s' is not writeable:.*", graphDirFile))
		})
	})
})
