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

		Context("when the graph directory is read-only", func() {
			var graphDir string

			BeforeEach(func() {
				var err error
				graphDir, err = ioutil.TempDir("", "read-only-graph-dir-test")
				Expect(err).NotTo(HaveOccurred())
				Expect(os.Chmod(graphDir, 0577)).To(Succeed())
				provider = sysinfo.NewProvider("/", graphDir)
			})

			AfterEach(func() {
				os.RemoveAll(graphDir)
			})

			It("should return an error", func() {
				err := provider.CheckHealth()
				Expect(err).To(BeAssignableToTypeOf(garden.NewUnrecoverableError("")))
				Expect(err.Error()).To(MatchRegexp("graph directory '%s' is not writeable:.*", graphDir))
			})
		})
	})
})
