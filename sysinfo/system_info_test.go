package sysinfo_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/sysinfo"

	"io/ioutil"
	"os"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SystemInfo", func() {
	var provider sysinfo.Provider

	Describe("TotalMemory", func() {
		BeforeEach(func() {
			provider = sysinfo.NewProvider("/", "/")
		})

		It("provides nonzero memory information", func() {
			totalMemory, err := provider.TotalMemory()
			Expect(err).ToNot(HaveOccurred())

			Expect(totalMemory).To(BeNumerically(">", 0))
		})
	})

	Describe("TotalDisk", func() {
		BeforeEach(func() {
			provider = sysinfo.NewProvider("/", "/")
		})

		It("provides nonzero disk information", func() {
			totalDisk, err := provider.TotalDisk()
			Expect(err).ToNot(HaveOccurred())

			Expect(totalDisk).To(BeNumerically(">", 0))
		})
	})

	Describe("CheckHealth", func() {
		Context("when the graph directory is read-write", func() {
			BeforeEach(func() {
				provider = sysinfo.NewProvider("/", "/tmp")
			})

			It("should not return an error", func() {
				Expect(provider.CheckHealth()).To(Succeed())
			})
		})

		Context("when the graph directory is not writable", func() {
			var graphDirFile string

			BeforeEach(func() {
				// Try to use a file as the graphdir -- this is certainly not a writable directory.
				tempFile, err := ioutil.TempFile("", "read-only-graph-dir-test")
				Expect(err).NotTo(HaveOccurred())

				graphDirFile = tempFile.Name()
				err = tempFile.Close()
				Expect(err).NotTo(HaveOccurred())

				provider = sysinfo.NewProvider("/", graphDirFile)
			})

			AfterEach(func() {
				os.RemoveAll(graphDirFile)
			})

			It("should return an error", func() {
				err := provider.CheckHealth()
				Expect(err).NotTo(BeNil())
				Expect(err).To(BeAssignableToTypeOf(garden.NewUnrecoverableError("")))
				Expect(err.Error()).To(MatchRegexp("graph directory '%s' is not writeable:.*", graphDirFile))
			})
		})
	})
})
