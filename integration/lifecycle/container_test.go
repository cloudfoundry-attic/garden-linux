package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Container operations", func() {
	var container garden.Container
	var args []string

	BeforeEach(func() {
		args = []string{}
	})

	JustBeforeEach(func() {
		client = startGarden(args...)
	})

	AfterEach(func() {
		if container != nil {
			Ω(client.Destroy(container.Handle())).Should(Succeed())
		}
	})

	Context("with a default rootfs", func() {
		It("the container is created successfully", func() {
			var err error

			container, err = client.Create(garden.ContainerSpec{RootFSPath: ""})
			Ω(err).ShouldNot(HaveOccurred())
		})

		PIt("the container is in a new namespace", func() {})
	})
})
