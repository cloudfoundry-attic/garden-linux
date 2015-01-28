package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Rootfs container create parameter", func() {
	var container garden.Container

	BeforeEach(func() {
		client = startGarden()
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
	})

	Context("with a docker rootfs URI", func() {
		Context("not containing a host", func() {
			It("the container is created successfully", func() {
				var err error

				container, err = client.Create(garden.ContainerSpec{RootFSPath: "docker:///busybox"})
				Ω(err).ShouldNot(HaveOccurred())
			})
		})

		Context("containing a host", func() {
			Context("which is valid", func() {
				It("the container is created successfully", func() {
					var err error

					container, err = client.Create(garden.ContainerSpec{RootFSPath: "docker://index.docker.io/busybox"})
					Ω(err).ShouldNot(HaveOccurred())
				})
			})

			Context("which is invalid", func() {
				It("the container is created successfully", func() {
					var err error

					container, err = client.Create(garden.ContainerSpec{RootFSPath: "docker://xindex.docker.io/busybox"})
					Ω(err).Should(MatchError("invalid docker url"))
				})
			})
		})
	})
})
