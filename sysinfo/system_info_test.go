package sysinfo_test

import (
	. "github.com/cloudfoundry-incubator/garden-linux/sysinfo"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SystemInfo", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewProvider("/")
	})

	It("provides nonzero memory and disk information", func() {
		totalMemory, err := provider.TotalMemory()
		Expect(err).ToNot(HaveOccurred())

		totalDisk, err := provider.TotalDisk()
		Expect(err).ToNot(HaveOccurred())

		Expect(totalMemory).To(BeNumerically(">", 0))
		Expect(totalDisk).To(BeNumerically(">", 0))
	})
})
