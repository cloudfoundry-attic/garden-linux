package lifecycle_test

import (
	"os"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ping", func() {
	Context("When a garden linux server is started", func() {
		BeforeEach(func() {
			client = startGarden()
		})

		It("should successfully ping", func() {
			Expect(client.Ping()).To(Succeed())
		})
	})

	Context("When a garden linux server is started with an untennable graph directory", func() {
		BeforeEach(func() {
			client = startGarden()
			Expect(os.MkdirAll(client.GraphPath, 0755)).To(Succeed())
			Expect(syscall.Mount("tmpfs", client.GraphPath, "tmpfs", syscall.MS_RDONLY, "")).To(Succeed())
		})

		AfterEach(func() {
			Expect(syscall.Unmount(client.GraphPath, 0)).To(Succeed())
		})

		It("should fail when pinged", func() {
			err := client.Ping()
			Expect(err).NotTo(BeNil())
			Expect(err).To(BeAssignableToTypeOf(garden.NewUnrecoverableError("")))
			Expect(err.Error()).To(MatchRegexp("graph directory '%s' is not writeable:.*", client.GraphPath))
		})
	})
})
