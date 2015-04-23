package system_test

import (
	"io"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("InitializerLinux", func() {
	Describe("Init", func() {
		It("sets the correct hostname", func() {
			stdout := gbytes.NewBuffer()
			Expect(
				runInContainer(io.MultiWriter(stdout, GinkgoWriter), GinkgoWriter,
					false, "fake_initializer", "potato", "hostname"),
			).To(Succeed())
			Expect(stdout).To(gbytes.Say("potato"))
		})
	})
})
